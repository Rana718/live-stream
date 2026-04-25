package stream

import (
	"context"
	"fmt"
	"live-platform/internal/database/db"
	"live-platform/internal/events"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	queries  *db.Queries
	producer *events.Producer
}

func NewService(pool *pgxpool.Pool, producer *events.Producer) *Service {
	return &Service{
		queries:  db.New(pool),
		producer: producer,
	}
}

type CreateStreamRequest struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	ScheduledAt time.Time `json:"scheduled_at"`
}

func (s *Service) CreateStream(ctx context.Context, instructorID uuid.UUID, req CreateStreamRequest) (*db.Stream, error) {
	streamKey := uuid.New().String()

	scheduledAt := pgtype.Timestamp{Time: req.ScheduledAt, Valid: true}

	stream, err := s.queries.CreateStream(ctx, db.CreateStreamParams{
		Title:        req.Title,
		Description:  pgtype.Text{String: req.Description, Valid: true},
		InstructorID: pgtype.UUID{Bytes: instructorID, Valid: true},
		StreamKey:    streamKey,
		ScheduledAt:  scheduledAt,
	})
	if err != nil {
		return nil, err
	}

	s.producer.PublishEvent(ctx, uuid.UUID(stream.ID.Bytes).String(), map[string]interface{}{
		"event":     "stream_created",
		"stream_id": uuid.UUID(stream.ID.Bytes).String(),
		"timestamp": time.Now(),
	})

	return &stream, nil
}

func (s *Service) StartStream(ctx context.Context, streamID uuid.UUID) error {
	pgUUID := pgtype.UUID{Bytes: streamID, Valid: true}
	stream, err := s.queries.StartStream(ctx, pgUUID)
	if err != nil {
		return err
	}

	s.producer.PublishEvent(ctx, streamID.String(), map[string]interface{}{
		"event":     "stream_started",
		"stream_id": uuid.UUID(stream.ID.Bytes).String(),
		"timestamp": time.Now(),
	})

	return nil
}

func (s *Service) EndStream(ctx context.Context, streamID uuid.UUID) error {
	pgUUID := pgtype.UUID{Bytes: streamID, Valid: true}
	stream, err := s.queries.EndStream(ctx, pgUUID)
	if err != nil {
		return err
	}

	s.producer.PublishEvent(ctx, streamID.String(), map[string]interface{}{
		"event":     "stream_ended",
		"stream_id": uuid.UUID(stream.ID.Bytes).String(),
		"timestamp": time.Now(),
	})

	return nil
}

func (s *Service) GetStream(ctx context.Context, streamID uuid.UUID) (*db.Stream, error) {
	pgUUID := pgtype.UUID{Bytes: streamID, Valid: true}
	stream, err := s.queries.GetStreamByID(ctx, pgUUID)
	if err != nil {
		return nil, err
	}
	return &stream, nil
}

func (s *Service) ListLiveStreams(ctx context.Context) ([]db.Stream, error) {
	return s.queries.ListLiveStreams(ctx)
}

func (s *Service) UpdateViewerCount(ctx context.Context, streamID uuid.UUID, count int32) error {
	return s.queries.UpdateViewerCount(ctx, db.UpdateViewerCountParams{
		ID:          pgtype.UUID{Bytes: streamID, Valid: true},
		ViewerCount: pgtype.Int4{Int32: count, Valid: true},
	})
}

func (s *Service) ValidateStreamKey(ctx context.Context, streamKey string) (*db.Stream, error) {
	stream, err := s.queries.GetStreamByKey(ctx, streamKey)
	if err != nil {
		return nil, fmt.Errorf("invalid stream key")
	}
	return &stream, nil
}

// StartStreamByKey starts a stream using its stream key (called by RTMP auth callback).
//
// Tenant-scoping: the key→stream lookup uses GetStreamByKey which RLS does
// not gate (the connection here has no app.tenant_id session var because
// the RTMP webhook is unauthenticated). We treat the lookup as authoritative
// — a key that doesn't resolve is an immediate 401. We then re-check the
// owning tenant's status: a suspended tenant must not be allowed to ingest
// new content even if their key is still valid.
func (s *Service) StartStreamByKey(ctx context.Context, streamKey string) (*db.Stream, error) {
	stream, err := s.queries.GetStreamByKey(ctx, streamKey)
	if err != nil {
		return nil, fmt.Errorf("invalid stream key: %v", err)
	}

	// Refuse to ingest for suspended tenants. The tenant lookup runs with
	// is_super_admin=true so the SELECT bypasses the per-tenant RLS policy.
	tenantID := uuid.UUID(stream.TenantID.Bytes)
	if tenantID != uuid.Nil {
		t, err := s.queries.GetTenantByID(ctx, stream.TenantID)
		if err == nil && t.Status != "active" {
			return nil, fmt.Errorf("tenant %s is %s — ingest refused", t.OrgCode, t.Status)
		}
	}

	updatedStream, err := s.queries.StartStream(ctx, stream.ID)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Stream started successfully: ID=%v, Status=%v\n", updatedStream.ID, updatedStream.Status)

	s.producer.PublishEvent(ctx, uuid.UUID(updatedStream.ID.Bytes).String(), map[string]interface{}{
		"event":      "stream_started",
		"stream_id":  uuid.UUID(updatedStream.ID.Bytes).String(),
		"stream_key": streamKey,
		"timestamp":  time.Now(),
	})

	return &updatedStream, nil
}

// EndStreamByKey ends a stream using its stream key (called by RTMP done callback)
func (s *Service) EndStreamByKey(ctx context.Context, streamKey string) (*db.Stream, error) {
	// Get stream by key first
	stream, err := s.queries.GetStreamByKey(ctx, streamKey)
	if err != nil {
		return nil, fmt.Errorf("invalid stream key")
	}

	// End the stream (update status to ended)
	updatedStream, err := s.queries.EndStream(ctx, stream.ID)
	if err != nil {
		return nil, err
	}

	s.producer.PublishEvent(ctx, uuid.UUID(updatedStream.ID.Bytes).String(), map[string]interface{}{
		"event":      "stream_ended",
		"stream_id":  uuid.UUID(updatedStream.ID.Bytes).String(),
		"stream_key": streamKey,
		"timestamp":  time.Now(),
	})

	return &updatedStream, nil
}
