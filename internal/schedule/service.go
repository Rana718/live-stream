// Package schedule manages recurring class schedules and the worker that
// materialises them into concrete `streams` rows.
//
// Tenant admins create a schedule once ("Hindi grammar, Mon+Wed at 18:00 IST,
// 90 min, course X, batch Y"). A daily worker walks every active schedule
// and pre-creates streams for the next 14 days. Students browsing /live
// see the upcoming classes immediately; the instructor doesn't have to
// remember to schedule each one individually.
//
// We deliberately don't model RRULEs or arbitrary calendar events — the
// pattern we see (weekday list + clock time + duration) covers every
// coaching-class case we've found, and trying to be a Calendar product
// would distract from teaching workflows.
package schedule

import (
	"context"
	"fmt"
	"strings"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// MaterialiseHorizon is how far into the future we pre-create
	// streams rows. Two weeks balances "students see the class on the
	// calendar" with "we don't fill the streams table with rows that
	// might be cancelled".
	MaterialiseHorizon = 14 * 24 * time.Hour

	// DefaultStreamKeyBytes — entropy for the rotating per-stream key.
	// 16 bytes hex-encoded → 32 chars, plenty.
	DefaultStreamKeyBytes = 16
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

// CreateInput is what the admin form posts. Validation happens in the
// handler — by the time we get here the input is structurally clean.
type CreateInput struct {
	InstructorID string  `json:"instructor_id" validate:"required,uuid"`
	CourseID     string  `json:"course_id"`
	BatchID      string  `json:"batch_id"`
	Title        string  `json:"title" validate:"required,min=1,max=200"`
	Description  string  `json:"description"`
	ByWeekday    []int16 `json:"by_weekday" validate:"required,min=1"`
	StartLocal   string  `json:"start_local" validate:"required"`
	DurationMin  int32   `json:"duration_min"`
	Timezone     string  `json:"timezone"`
	StartsOn     string  `json:"starts_on"`
	EndsOn       string  `json:"ends_on"`
}

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, in CreateInput) (*db.ClassSchedule, error) {
	if in.DurationMin <= 0 {
		in.DurationMin = 60
	}
	if in.Timezone == "" {
		in.Timezone = "Asia/Kolkata"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return nil, fmt.Errorf("invalid timezone")
	}
	// `start_local` is a "HH:MM" wall-clock string; sanity-check format.
	if _, err := time.Parse("15:04", in.StartLocal); err != nil {
		return nil, fmt.Errorf("start_local must be HH:MM (24h)")
	}
	for _, d := range in.ByWeekday {
		if d < 1 || d > 7 {
			return nil, fmt.Errorf("by_weekday values must be 1..7 (ISO)")
		}
	}

	instructor, err := uuid.Parse(in.InstructorID)
	if err != nil {
		return nil, fmt.Errorf("instructor_id invalid")
	}

	startsOn := pgtype.Date{Valid: false}
	if in.StartsOn != "" {
		t, err := time.Parse("2006-01-02", in.StartsOn)
		if err != nil {
			return nil, fmt.Errorf("starts_on must be YYYY-MM-DD")
		}
		startsOn = pgtype.Date{Time: t, Valid: true}
	}
	endsOn := pgtype.Date{Valid: false}
	if in.EndsOn != "" {
		t, err := time.Parse("2006-01-02", in.EndsOn)
		if err != nil {
			return nil, fmt.Errorf("ends_on must be YYYY-MM-DD")
		}
		endsOn = pgtype.Date{Time: t, Valid: true}
	}

	row, err := s.q.CreateClassSchedule(ctx, db.CreateClassScheduleParams{
		TenantID:     utils.UUIDToPg(tenantID),
		InstructorID: utils.UUIDToPg(instructor),
		CourseID:     parseOptionalUUID(in.CourseID),
		BatchID:      parseOptionalUUID(in.BatchID),
		Title:        in.Title,
		Description:  pgtype.Text{String: in.Description, Valid: in.Description != ""},
		ByWeekday:    in.ByWeekday,
		StartLocal:   in.StartLocal,
		DurationMin:  in.DurationMin,
		Timezone:     in.Timezone,
		StartsOn:     startsOn,
		EndsOn:       endsOn,
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func parseOptionalUUID(s string) pgtype.UUID {
	if s == "" {
		return pgtype.UUID{}
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func (s *Service) List(ctx context.Context, activeOnly *bool, limit, offset int32) ([]db.ClassSchedule, error) {
	var p pgtype.Bool
	if activeOnly != nil {
		p = pgtype.Bool{Bool: *activeOnly, Valid: true}
	}
	return s.q.ListClassSchedules(ctx, db.ListClassSchedulesParams{
		ActiveFilter: p,
		Limit:        limit,
		Offset:       offset,
	})
}

func (s *Service) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	return s.q.SetClassScheduleActive(ctx, db.SetClassScheduleActiveParams{
		ID:       utils.UUIDToPg(id),
		IsActive: active,
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteClassSchedule(ctx, utils.UUIDToPg(id))
}

// nextOccurrences returns the upcoming start-times for a schedule within
// the materialise horizon. Pure function over the schedule shape — no IO,
// trivially testable.
func nextOccurrences(s *db.ClassSchedule, horizon time.Duration) ([]time.Time, error) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return nil, err
	}
	hh, mm, _ := strings.Cut(s.StartLocal, ":")
	hour, err1 := atoi(hh)
	min, err2 := atoi(mm)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("malformed start_local %q", s.StartLocal)
	}

	// Build a quick set of days-of-week we care about.
	want := map[time.Weekday]bool{}
	for _, d := range s.ByWeekday {
		// ISO 1..7 (Mon=1 .. Sun=7) → time.Weekday (Sun=0 .. Sat=6).
		var w time.Weekday
		switch d {
		case 7:
			w = time.Sunday
		default:
			w = time.Weekday(d)
		}
		want[w] = true
	}

	// Window is [today, today+horizon] in the schedule's local TZ.
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	end := now.Add(horizon)

	var out []time.Time
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		if !want[d.Weekday()] {
			continue
		}
		// Skip if before schedule's starts_on or after ends_on.
		if s.StartsOn.Valid && d.Before(s.StartsOn.Time) {
			continue
		}
		if s.EndsOn.Valid && d.After(s.EndsOn.Time) {
			continue
		}
		when := time.Date(d.Year(), d.Month(), d.Day(), hour, min, 0, 0, loc)
		// Don't materialise occurrences that already started > 1h ago —
		// they're effectively missed for this run.
		if when.Before(now.Add(-1 * time.Hour)) {
			continue
		}
		out = append(out, when.UTC())
	}
	return out, nil
}

// atoi is a tiny stdlib-free integer parser to avoid pulling strconv into
// a hot path. Schedule times are always 2-digit, but we accept 1-digit
// for safety.
func atoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a digit: %q", c)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
