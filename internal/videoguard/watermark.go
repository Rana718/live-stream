package videoguard

import (
	"context"
	"fmt"
	"strings"
)

// WatermarkSpec is the payload our FFmpeg worker (separate sidecar process)
// reads off Kafka. The worker re-runs FFmpeg with a `drawtext` overlay
// containing the student's phone number, then publishes the result back
// into the recordings bucket as a per-student variant.
//
// We don't compile FFmpeg into the API — the binary is huge, the spawn
// is slow, and shipping it inside the Go process would couple our deploy
// cadence to libavcodec security CVEs. Keeping the worker external also
// makes scaling trivial: more transcode load = spin more sidecars.
type WatermarkSpec struct {
	SourceURL      string `json:"source_url"`       // signed MinIO URL of the master variant
	OutputBucket   string `json:"output_bucket"`    // typically `recordings`
	OutputKey      string `json:"output_key"`       // e.g. `<recording_id>/<student_id>.mp4`
	WatermarkText  string `json:"watermark_text"`   // student phone, partial-redacted
	OpacityPercent int    `json:"opacity_percent"`  // 25–60 sweet spot
	Position       string `json:"position"`         // "bottom-right" | "top-left" | etc.
}

// BuildSpec returns a watermark spec for one (recording, student) pair.
// The phone number is partial-redacted (XXXXXX1234) so a screen-recorder
// leak still de-anonymises the student but doesn't fully expose their
// number to other viewers.
func BuildSpec(sourceURL, recordingID, studentID, studentPhone string) WatermarkSpec {
	redacted := redactPhone(studentPhone)
	return WatermarkSpec{
		SourceURL:      sourceURL,
		OutputBucket:   "recordings",
		OutputKey:      fmt.Sprintf("%s/%s.mp4", recordingID, studentID),
		WatermarkText:  redacted + " · " + studentID[:8],
		OpacityPercent: 35,
		Position:       "bottom-right",
	}
}

func redactPhone(phone string) string {
	p := strings.TrimPrefix(phone, "+")
	if len(p) <= 4 {
		return p
	}
	return strings.Repeat("X", len(p)-4) + p[len(p)-4:]
}

// Dispatcher hands the watermark spec to whatever transport the worker
// listens on. We model it as an interface so the API can stay agnostic:
// today it's Kafka, tomorrow it could be Redis Streams or a job table.
type Dispatcher interface {
	Submit(ctx context.Context, spec WatermarkSpec) error
}
