// Package schedule manages recurring class schedules. A tenant admin creates
// a schedule once ("Mon+Wed 18:00 IST, 90 min, course X"); the schedule
// worker (cmd/scheduleworker) materialises them into live_sessions rows.
// Pattern is weekday-list + wall-clock + duration — no RRULEs.
package schedule

import (
	"context"
	"fmt"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MaterialiseHorizon = 14 * 24 * time.Hour

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

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

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, in CreateInput) (db.CreateClassScheduleRow, error) {
	if in.DurationMin <= 0 {
		in.DurationMin = 60
	}
	if in.Timezone == "" {
		in.Timezone = "Asia/Kolkata"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return db.CreateClassScheduleRow{}, fmt.Errorf("invalid timezone")
	}
	if _, err := time.Parse("15:04", in.StartLocal); err != nil {
		return db.CreateClassScheduleRow{}, fmt.Errorf("start_local must be HH:MM (24h)")
	}
	for _, d := range in.ByWeekday {
		if d < 1 || d > 7 {
			return db.CreateClassScheduleRow{}, fmt.Errorf("by_weekday values must be 1..7 (ISO)")
		}
	}
	instructor, err := uuid.Parse(in.InstructorID)
	if err != nil {
		return db.CreateClassScheduleRow{}, fmt.Errorf("instructor_id invalid")
	}

	parseDate := func(v string) (pgtype.Date, error) {
		if v == "" {
			return pgtype.Date{}, nil
		}
		t, e := time.Parse("2006-01-02", v)
		if e != nil {
			return pgtype.Date{}, fmt.Errorf("date must be YYYY-MM-DD")
		}
		return pgtype.Date{Time: t, Valid: true}, nil
	}
	startsOn, err := parseDate(in.StartsOn)
	if err != nil {
		return db.CreateClassScheduleRow{}, err
	}
	endsOn, err := parseDate(in.EndsOn)
	if err != nil {
		return db.CreateClassScheduleRow{}, err
	}

	return s.q.CreateClassSchedule(ctx, db.CreateClassScheduleParams{
		TenantID:     utils.UUIDToPg(tenantID),
		InstructorID: utils.UUIDToPg(instructor),
		CourseID:     parseOptionalUUID(in.CourseID),
		BatchID:      parseOptionalUUID(in.BatchID),
		Title:        in.Title,
		Description:  pgtype.Text{String: in.Description, Valid: in.Description != ""},
		ByWeekday:    in.ByWeekday,
		StartLocal:   in.StartLocal,
		DurationMin:  pgtype.Int4{Int32: in.DurationMin, Valid: true},
		Timezone:     pgtype.Text{String: in.Timezone, Valid: true},
		StartsOn:     startsOn,
		EndsOn:       endsOn,
	})
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]db.ListClassSchedulesRow, error) {
	return s.q.ListClassSchedules(ctx, utils.UUIDToPg(tenantID))
}

func (s *Service) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	return s.q.SetScheduleActive(ctx, db.SetScheduleActiveParams{
		ID: utils.UUIDToPg(id), IsActive: active,
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteClassSchedule(ctx, utils.UUIDToPg(id))
}
