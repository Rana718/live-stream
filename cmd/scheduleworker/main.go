// scheduleworker is the daily cron that materialises recurring class
// schedules into concrete streams rows. It runs once per invocation and
// exits — Kubernetes CronJob, GitHub Actions cron, or `cron.d` on a VPS
// can drive it. There's no internal scheduler so the binary stays
// dead-simple and we don't have to reason about clock-drift across
// replicas.
//
// Algorithm:
//   1. Load every schedule that's active, in-window, and either un-
//      materialised or last-touched >12h ago.
//   2. For each, compute the next 14 days of occurrences in the
//      schedule's local timezone.
//   3. Insert a `streams` row per occurrence (idempotent — the
//      `(schedule_id, scheduled_at)` uniqueness check skips dupes).
//   4. Touch `last_materialised_at` so we don't re-process for 12h.
//
// We intentionally don't run as a long-lived process — the work is
// fundamentally periodic and any state we'd hold in memory we'd just
// have to re-read from Postgres anyway.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"live-platform/internal/config"
	"live-platform/internal/database"
	"live-platform/internal/database/db"
	"live-platform/internal/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// horizon is how far ahead we materialise. 14 days matches the docstring
// in internal/schedule. Tweak both together if you change one.
const horizon = 14 * 24 * time.Hour

// batchSize caps schedules per run. The worker runs daily so this is the
// max we'll see per tick; well above the 100s of schedules a tenant
// typically has.
const batchSize = 1000

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config load:", err)
		os.Exit(1)
	}
	log := logger.Init(cfg.Logging.Level, cfg.Logging.Format)

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := database.NewPostgresPool(&cfg.Database)
	if err != nil {
		log.Error("pg connect", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	if err := run(ctx, pool, log); err != nil {
		log.Error("run failed", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	// Materialiser bypasses RLS — schedules belong to many tenants and
	// we walk them all. SuperAdmin session var sets that bypass.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx,
		"SELECT set_config('app.is_super_admin', 'true', false)"); err != nil {
		return err
	}

	q := db.New(conn)
	schedules, err := q.ListActiveSchedulesNeedingMaterialisation(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	log.Info("schedule worker tick",
		slog.Int("schedules", len(schedules)),
		slog.Duration("horizon", horizon))

	created, skipped, errs := 0, 0, 0
	for _, s := range schedules {
		c, sk, e := materialise(ctx, q, s, log)
		created += c
		skipped += sk
		errs += e
	}
	log.Info("schedule worker done",
		slog.Int("streams_created", created),
		slog.Int("streams_skipped", skipped),
		slog.Int("errors", errs))
	return nil
}

func materialise(ctx context.Context, q *db.Queries, s db.ClassSchedule, log *slog.Logger) (created, skipped, errs int) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		log.Warn("bad tz on schedule",
			slog.String("schedule_id", uuid.UUID(s.ID.Bytes).String()),
			slog.String("tz", s.Timezone))
		errs++
		return
	}
	hour, minute, ok := parseHHMM(s.StartLocal)
	if !ok {
		errs++
		return
	}

	// Build a fast set of weekdays we materialise.
	want := map[time.Weekday]bool{}
	for _, d := range s.ByWeekday {
		w := time.Weekday(d) // ISO 1..6 maps directly; 7 is Sunday=0.
		if d == 7 {
			w = time.Sunday
		}
		want[w] = true
	}

	now := time.Now().In(loc)
	end := now.Add(horizon)

	for day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc); day.Before(end); day = day.AddDate(0, 0, 1) {
		if !want[day.Weekday()] {
			continue
		}
		if s.StartsOn.Valid && day.Before(s.StartsOn.Time) {
			continue
		}
		if s.EndsOn.Valid && day.After(s.EndsOn.Time) {
			continue
		}
		when := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc).UTC()
		// Skip occurrences that already started >1h ago — missed for
		// this run, no point creating a "live" row in the past.
		if when.Before(now.Add(-1 * time.Hour)) {
			continue
		}

		whenPg := pgtype.Timestamp{Time: when, Valid: true}
		exists, err := q.StreamExistsForSchedule(ctx, db.StreamExistsForScheduleParams{
			ScheduleID:  s.ID,
			ScheduledAt: whenPg,
		})
		if err != nil {
			errs++
			continue
		}
		if exists {
			skipped++
			continue
		}

		_, err = q.CreateScheduledStream(ctx, db.CreateScheduledStreamParams{
			TenantID:     s.TenantID,
			InstructorID: s.InstructorID,
			Title:        s.Title,
			Description:  s.Description,
			ScheduledAt:  whenPg,
			ScheduleID:   s.ID,
		})
		if err != nil {
			log.Warn("create stream failed",
				slog.String("schedule_id", uuid.UUID(s.ID.Bytes).String()),
				slog.String("err", err.Error()))
			errs++
			continue
		}
		created++
	}

	if err := q.TouchScheduleMaterialised(ctx, s.ID); err != nil {
		log.Warn("touch materialised failed",
			slog.String("schedule_id", uuid.UUID(s.ID.Bytes).String()))
	}
	return
}

func parseHHMM(s string) (h, m int, ok bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return
	}
	h, err1 := atoi(parts[0])
	m, err2 := atoi(parts[1])
	return h, m, err1 == nil && err2 == nil && h >= 0 && h < 24 && m >= 0 && m < 60
}

func atoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not digit")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
