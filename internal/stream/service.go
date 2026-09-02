// Package stream — schema-v2. The `streams` table became `live_sessions`
// (ingest_key was stream_key, peak_viewers was viewer_count, session_status
// enum scheduled|live|ended|cancelled).
package stream

import (
	"context"
	"fmt"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/events"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q        *db.Queries
	producer *events.Producer
}

func NewService(pool *pgxpool.Pool, producer *events.Producer) *Service {
	return &Service{q: db.New(pool), producer: producer}
}

func (s *Service) emit(ctx context.Context, id string, m map[string]interface{}) {
	if s.producer != nil {
		_ = s.producer.PublishEvent(ctx, id, m)
	}
}

type CreateStreamRequest struct {
	CourseID    *uuid.UUID `json:"course_id"`
	BatchID     *uuid.UUID `json:"batch_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	ScheduledAt time.Time  `json:"scheduled_at"`
}

type Session struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenant_id"`
	CourseID     string     `json:"course_id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Status       string     `json:"status"`
	IngestKey    string     `json:"ingest_key"`
	StreamKey    string     `json:"stream_key"` // legacy alias
	ScheduledAt  *time.Time `json:"scheduled_at"`
	StartedAt    *time.Time `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at"`
	PeakViewers  int32      `json:"peak_viewers"`
	ViewerCount  int32      `json:"viewer_count"` // legacy alias
	InstructorID string     `json:"instructor_id"`
	HLSURL       string     `json:"hls_url"`
	RTMPURL      string     `json:"rtmp_url"`
}

func hlsURL(key string) string  { return "http://localhost:8080/hls/" + key + ".m3u8" }
func rtmpURL(key string) string { return "rtmp://localhost:1935/live/" + key }

func tptr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func (s *Service) CreateStream(ctx context.Context, tenantID, instructorID uuid.UUID, req CreateStreamRequest) (Session, error) {
	key := uuid.New().String()
	var sched pgtype.Timestamptz
	if !req.ScheduledAt.IsZero() {
		sched = pgtype.Timestamptz{Time: req.ScheduledAt, Valid: true}
	}
	row, err := s.q.CreateLiveSession(ctx, db.CreateLiveSessionParams{
		TenantID:       utils.UUIDToPg(tenantID),
		CourseID:       utils.UUIDPtrToPg(req.CourseID),
		BatchID:        utils.UUIDPtrToPg(req.BatchID),
		InstructorID:   utils.UUIDToPg(instructorID),
		Title:          req.Title,
		Description:    pgtype.Text{String: req.Description, Valid: req.Description != ""},
		IngestKey:      key,
		ScheduledStart: sched,
	})
	if err != nil {
		return Session{}, err
	}
	id := utils.UUIDFromPg(row.ID)
	s.emit(ctx, id, map[string]interface{}{"event": "stream_created", "stream_id": id, "timestamp": time.Now()})
	return Session{
		ID: id, TenantID: utils.UUIDFromPg(row.TenantID), CourseID: utils.UUIDFromPg(row.CourseID),
		Title: row.Title, Status: string(row.Status), IngestKey: row.IngestKey, StreamKey: row.IngestKey,
		ScheduledAt: tptr(row.ScheduledStart), InstructorID: instructorID.String(),
		HLSURL: hlsURL(row.IngestKey), RTMPURL: rtmpURL(row.IngestKey),
	}, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Session, error) {
	r, err := s.q.GetLiveSession(ctx, utils.UUIDToPg(id))
	if err != nil {
		return Session{}, err
	}
	return Session{
		ID: utils.UUIDFromPg(r.ID), TenantID: utils.UUIDFromPg(r.TenantID),
		CourseID: utils.UUIDFromPg(r.CourseID), Title: r.Title,
		Description: utils.TextFromPg(r.Description), Status: string(r.Status),
		IngestKey: r.IngestKey, StreamKey: r.IngestKey,
		ScheduledAt: tptr(r.ScheduledStart), StartedAt: tptr(r.ActualStart), EndedAt: tptr(r.ActualEnd),
		PeakViewers: r.PeakViewers, ViewerCount: r.PeakViewers,
		InstructorID: utils.UUIDFromPg(r.InstructorID),
		HLSURL:       hlsURL(r.IngestKey), RTMPURL: rtmpURL(r.IngestKey),
	}, nil
}

func (s *Service) ListLive(ctx context.Context, tenantID uuid.UUID) ([]Session, error) {
	if tenantID == uuid.Nil {
		return []Session{}, nil
	}
	rows, err := s.q.ListLiveSessions(ctx, db.ListLiveSessionsParams{
		TenantID: utils.UUIDToPg(tenantID),
		Statuses: []db.SessionStatus{db.SessionStatus("live"), db.SessionStatus("scheduled")},
		Limit:    100, Offset: 0,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, Session{
			ID: utils.UUIDFromPg(r.ID), CourseID: utils.UUIDFromPg(r.CourseID),
			Title: r.Title, Status: string(r.Status),
			ScheduledAt: tptr(r.ScheduledStart), StartedAt: tptr(r.ActualStart),
			PeakViewers: r.PeakViewers, ViewerCount: r.PeakViewers,
		})
	}
	return out, nil
}

func (s *Service) Start(ctx context.Context, id uuid.UUID) error {
	if _, err := s.q.StartLiveSession(ctx, utils.UUIDToPg(id)); err != nil {
		return err
	}
	s.emit(ctx, id.String(), map[string]interface{}{"event": "stream_started", "stream_id": id.String(), "timestamp": time.Now()})
	return nil
}

func (s *Service) End(ctx context.Context, id uuid.UUID) error {
	if _, err := s.q.EndLiveSession(ctx, utils.UUIDToPg(id)); err != nil {
		return err
	}
	s.emit(ctx, id.String(), map[string]interface{}{"event": "stream_ended", "stream_id": id.String(), "timestamp": time.Now()})
	return nil
}

func (s *Service) UpdateViewerCount(ctx context.Context, id uuid.UUID, count int32) error {
	return s.q.SetLiveSessionPeakViewers(ctx, db.SetLiveSessionPeakViewersParams{
		ID: utils.UUIDToPg(id), PeakViewers: count,
	})
}

// StartStreamByKey / EndStreamByKey — called by the unauthenticated RTMP
// webhook. The key lookup bypasses RLS; a suspended tenant is refused.
func (s *Service) StartStreamByKey(ctx context.Context, key string) (Session, error) {
	r, err := s.q.GetLiveSessionByIngestKey(ctx, key)
	if err != nil {
		return Session{}, fmt.Errorf("invalid stream key: %v", err)
	}
	if r.TenantID.Valid {
		if t, e := s.q.GetTenantByID(ctx, r.TenantID); e == nil && string(t.Status) != "active" {
			return Session{}, fmt.Errorf("tenant %s is %s — ingest refused", t.OrgCode, t.Status)
		}
	}
	id := uuid.UUID(r.ID.Bytes)
	if err := s.Start(ctx, id); err != nil {
		return Session{}, err
	}
	return Session{ID: id.String(), IngestKey: key, StreamKey: key, Status: "live"}, nil
}

func (s *Service) EndStreamByKey(ctx context.Context, key string) (Session, error) {
	r, err := s.q.GetLiveSessionByIngestKey(ctx, key)
	if err != nil {
		return Session{}, fmt.Errorf("invalid stream key")
	}
	id := uuid.UUID(r.ID.Bytes)
	if err := s.End(ctx, id); err != nil {
		return Session{}, err
	}
	return Session{ID: id.String(), IngestKey: key, StreamKey: key, Status: "ended"}, nil
}

func (s *Service) ValidateStreamKey(ctx context.Context, key string) (Session, error) {
	r, err := s.q.GetLiveSessionByIngestKey(ctx, key)
	if err != nil {
		return Session{}, fmt.Errorf("invalid stream key")
	}
	return Session{ID: utils.UUIDFromPg(r.ID), IngestKey: key, StreamKey: key, Status: string(r.Status)}, nil
}
