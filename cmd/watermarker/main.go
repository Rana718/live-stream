// watermarker is the FFmpeg sidecar that consumes WatermarkSpec messages
// from Kafka, runs `ffmpeg drawtext` to overlay a per-student identifier
// onto the master recording, and uploads the resulting MP4 back to MinIO.
//
// Why a sidecar instead of running ffmpeg in-process from the API:
//   - libavcodec ships its own CVE cadence; we don't want our API
//     redeployed every time a codec issue lands.
//   - FFmpeg invocations are CPU-bound and can run for minutes per
//     recording. The API's request-handler model is the wrong shape.
//   - Scaling the sidecar horizontally is trivial (consumer group =
//     more replicas == more parallel transcodes).
//
// Wire format: a JSON-encoded videoguard.WatermarkSpec on a Kafka topic.
// The producer side lives in the API (videoguard.Dispatcher).
//
// Process model: read message → fetch source → ffmpeg → upload → commit.
// We do not retry inside the worker. If ffmpeg or the upload fails, the
// message stays uncommitted and Kafka redelivers after the consumer's
// session timeout. Idempotency at the destination (overwrite on PUT) is
// what makes this safe.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"live-platform/internal/config"
	"live-platform/internal/events"
	"live-platform/internal/logger"
	"live-platform/internal/storage"
	"live-platform/internal/videoguard"

	"github.com/minio/minio-go/v7"
)

const (
	// consumerGroup keeps multiple worker replicas from double-processing
	// the same WatermarkSpec — Kafka assigns each partition to one member.
	consumerGroup = "watermarker"

	// jobTimeout caps a single transcode. Most class recordings are
	// 60–90 minutes; ffmpeg drawtext on H.264 runs ~2x realtime on a
	// modern x86 box, so 30 minutes is a generous ceiling.
	jobTimeout = 30 * time.Minute

	// downloadTimeout is the budget for fetching the master variant from
	// MinIO before we even start ffmpeg.
	downloadTimeout = 5 * time.Minute
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config load:", err)
		os.Exit(1)
	}
	log := logger.Init(cfg.Logging.Level, cfg.Logging.Format)

	// Sanity-check ffmpeg is on PATH so we fail fast instead of per-job.
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Error("ffmpeg not on PATH — install ffmpeg in the worker image",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	mc, err := storage.NewMinIOClient(&cfg.MinIO)
	if err != nil {
		log.Error("minio init", slog.String("err", err.Error()))
		os.Exit(1)
	}

	consumer := events.NewConsumer(&cfg.Kafka, consumerGroup)
	defer consumer.Close()

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Info("watermarker started",
		slog.String("topic", cfg.Kafka.Topic),
		slog.String("group", consumerGroup))

	for {
		msg, err := consumer.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Info("shutdown")
				return
			}
			log.Warn("kafka read", slog.String("err", err.Error()))
			continue
		}

		// We share the API's main events topic, so most messages on it
		// will be ordinary domain events (course.purchased etc.) rather
		// than WatermarkSpec payloads. The cheap discriminator is the
		// presence of the source_url field — domain Events don't have it.
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(msg.Value, &probe); err != nil {
			continue
		}
		if _, ok := probe["source_url"]; !ok {
			continue
		}

		var spec videoguard.WatermarkSpec
		if err := json.Unmarshal(msg.Value, &spec); err != nil {
			log.Warn("decode WatermarkSpec",
				slog.String("err", err.Error()),
				slog.String("offset", fmt.Sprint(msg.Offset)))
			continue
		}

		jobCtx, jobCancel := context.WithTimeout(ctx, jobTimeout)
		if err := process(jobCtx, log, mc, spec); err != nil {
			log.Error("watermark job failed",
				slog.String("output_key", spec.OutputKey),
				slog.String("err", err.Error()))
		} else {
			log.Info("watermark job done",
				slog.String("output_key", spec.OutputKey))
		}
		jobCancel()
	}
}

func process(ctx context.Context, log *slog.Logger, mc *storage.MinIOClient, spec videoguard.WatermarkSpec) error {
	// Per-job scratch dir so concurrent jobs in the same replica don't
	// collide on filenames. We tear it down on return — TempDir is RAM-
	// backed on most container images so this is cheap.
	work, err := os.MkdirTemp("", "wm-*")
	if err != nil {
		return fmt.Errorf("tempdir: %w", err)
	}
	defer os.RemoveAll(work)

	src := filepath.Join(work, "in.mp4")
	out := filepath.Join(work, "out.mp4")

	if err := download(ctx, spec.SourceURL, src); err != nil {
		return fmt.Errorf("fetch source: %w", err)
	}

	if err := runFFmpeg(ctx, src, out, spec); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}

	f, err := os.Open(out)
	if err != nil {
		return fmt.Errorf("open output: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat output: %w", err)
	}

	// EnsureBucket is idempotent — first call per bucket wins, rest no-op.
	if err := mc.EnsureBucket(ctx, spec.OutputBucket); err != nil {
		return fmt.Errorf("ensure bucket: %w", err)
	}
	// MinIO's PutObject overwrites on key collision, so this is also our
	// retry-safety guarantee: re-running the same job clobbers the same
	// key with byte-identical output.
	if _, err := mc.Raw().PutObject(ctx, spec.OutputBucket, spec.OutputKey,
		f, st.Size(), minio.PutObjectOptions{ContentType: "video/mp4"}); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return nil
}

func download(ctx context.Context, url, dest string) error {
	dctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(dctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("source HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func runFFmpeg(ctx context.Context, src, out string, spec videoguard.WatermarkSpec) error {
	// drawtext positions are ffmpeg expressions over (W,H,text_w,text_h).
	// Each branch leaves a 24px margin from the relevant edge.
	x, y := "w-text_w-24", "h-text_h-24"
	switch spec.Position {
	case "top-left":
		x, y = "24", "24"
	case "top-right":
		x, y = "w-text_w-24", "24"
	case "bottom-left":
		x, y = "24", "h-text_h-24"
	case "bottom-right":
		// default
	}
	op := spec.OpacityPercent
	if op <= 0 || op > 100 {
		op = 35
	}
	// Escape ":" and "'" since drawtext uses them as separators.
	text := escapeDrawtext(spec.WatermarkText)

	// We re-encode (no -c copy) because drawtext is a pixel-level filter.
	// CRF 23 is the FFmpeg recommended visually-lossless ceiling; preset
	// veryfast trades ~5% size for ~3x speed which matters at scale.
	filter := fmt.Sprintf(
		"drawtext=text='%s':x=%s:y=%s:fontsize=24:fontcolor=white@%.2f:box=1:boxcolor=black@%.2f:boxborderw=6",
		text, x, y, float64(op)/100.0, float64(op)/100.0/2.0,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-i", src,
		"-vf", filter,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-c:a", "copy",
		"-movflags", "+faststart",
		out,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func escapeDrawtext(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' || c == ':' || c == '\\' {
			out = append(out, '\\')
		}
		out = append(out, c)
	}
	return string(out)
}
