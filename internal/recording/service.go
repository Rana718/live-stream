package recording

import (
	"context"
	"live-platform/internal/database/db"
	"live-platform/internal/events"
	"live-platform/internal/storage"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	queries  *db.Queries
	storage  *storage.MinIOClient
	producer *events.Producer
}

func NewService(pool *pgxpool.Pool, storage *storage.MinIOClient, producer *events.Producer) *Service {
	return &Service{
		queries:  db.New(pool),
		storage:  storage,
		producer: producer,
	}
}

func (s *Service) CreateRecording(ctx context.Context, streamID uuid.UUID, filePath string) (*db.Recording, error) {
	status := "processing"
	recording, err := s.queries.CreateRecording(ctx, db.CreateRecordingParams{
		StreamID: pgtype.UUID{Bytes: streamID, Valid: true},
		FilePath: filePath,
		Status:   pgtype.Text{String: status, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	s.producer.PublishEvent(ctx, uuid.UUID(recording.ID.Bytes).String(), map[string]interface{}{
		"event":        "recording_created",
		"recording_id": uuid.UUID(recording.ID.Bytes).String(),
		"stream_id":    streamID.String(),
		"timestamp":    time.Now(),
	})

	return &recording, nil
}

func (s *Service) GetRecording(ctx context.Context, recordingID uuid.UUID) (*db.Recording, error) {
	pgUUID := pgtype.UUID{Bytes: recordingID, Valid: true}
	recording, err := s.queries.GetRecordingByID(ctx, pgUUID)
	if err != nil {
		return nil, err
	}
	return &recording, nil
}

func (s *Service) GetRecordingsByStream(ctx context.Context, streamID uuid.UUID) ([]db.Recording, error) {
	pgUUID := pgtype.UUID{Bytes: streamID, Valid: true}
	return s.queries.GetRecordingsByStreamID(ctx, pgUUID)
}

func (s *Service) UpdateRecordingStatus(ctx context.Context, recordingID uuid.UUID, status string) error {
	_, err := s.queries.UpdateRecordingStatus(ctx, db.UpdateRecordingStatusParams{
		ID:     pgtype.UUID{Bytes: recordingID, Valid: true},
		Status: pgtype.Text{String: status, Valid: true},
	})
	return err
}

func (s *Service) GetRecordingURL(ctx context.Context, recordingID uuid.UUID) (string, error) {
	pgUUID := pgtype.UUID{Bytes: recordingID, Valid: true}
	recording, err := s.queries.GetRecordingByID(ctx, pgUUID)
	if err != nil {
		return "", err
	}

	return s.storage.GetFileURL(recording.FilePath)
}
