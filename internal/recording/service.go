// Package recording — schema-v2. recordings.stream_id → session_id,
// file_path → file_key, status is the recording_status enum
// (queued|processing|ready|failed). Recordings hang off live_sessions.
package recording

import (
	"context"
	"fmt"
	"io"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/events"
	"live-platform/internal/storage"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q        *db.Queries
	storage  *storage.MinIOClient
	producer *events.Producer
}

func NewService(pool *pgxpool.Pool, st *storage.MinIOClient, producer *events.Producer) *Service {
	return &Service{q: db.New(pool), storage: st, producer: producer}
}

func (s *Service) emit(ctx context.Context, id string, m map[string]interface{}) {
	if s.producer != nil {
		_ = s.producer.PublishEvent(ctx, id, m)
	}
}

type Recording struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	CourseID     string `json:"course_id,omitempty"`
	Title        string `json:"title,omitempty"`
	FileKey      string `json:"file_key"`
	Status       string `json:"status"`
	DurationSec  int32  `json:"duration_seconds"`
	ThumbnailURL string `json:"thumbnail_url"`
	CreatedAt    any    `json:"created_at"`
	PlayURL      string `json:"play_url,omitempty"`
}

func (s *Service) UploadRecording(ctx context.Context, tenantID, sessionID uuid.UUID, key string, reader io.Reader, size int64) (Recording, error) {
	if err := s.storage.UploadFile(ctx, key, reader, size, "video/webm"); err != nil {
		return Recording{}, err
	}
	row, err := s.q.CreateRecording(ctx, db.CreateRecordingParams{
		TenantID:  utils.UUIDToPg(tenantID),
		SessionID: utils.UUIDToPg(sessionID),
		FileKey:   pgtype.Text{String: key, Valid: true},
		Status:    db.NullRecordingStatus{RecordingStatus: db.RecordingStatus("ready"), Valid: true},
	})
	if err != nil {
		return Recording{}, err
	}
	_, _ = s.q.UpdateRecording(ctx, db.UpdateRecordingParams{
		ID:       row.ID,
		FileSize: pgtype.Int8{Int64: size, Valid: true},
		Status:   db.NullRecordingStatus{RecordingStatus: db.RecordingStatus("ready"), Valid: true},
	})
	id := utils.UUIDFromPg(row.ID)
	s.emit(ctx, id, map[string]interface{}{"event": "recording_uploaded", "recording_id": id, "session_id": sessionID.String(), "timestamp": time.Now()})
	return Recording{ID: id, SessionID: sessionID.String(), FileKey: key, Status: "ready", CreatedAt: row.CreatedAt.Time}, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Recording, error) {
	r, err := s.q.GetRecording(ctx, utils.UUIDToPg(id))
	if err != nil {
		return Recording{}, err
	}
	return Recording{
		ID: utils.UUIDFromPg(r.ID), SessionID: utils.UUIDFromPg(r.SessionID),
		FileKey: utils.TextFromPg(r.FileKey), Status: string(r.Status),
		DurationSec: utils.Int4FromPg(r.DurationSec), ThumbnailURL: utils.TextFromPg(r.ThumbnailUrl),
	}, nil
}

func (s *Service) GetURL(ctx context.Context, id uuid.UUID) (string, error) {
	r, err := s.q.GetRecording(ctx, utils.UUIDToPg(id))
	if err != nil {
		return "", err
	}
	return s.storage.GetFileURL(utils.TextFromPg(r.FileKey))
}

func (s *Service) BySession(ctx context.Context, sessionID uuid.UUID) ([]Recording, error) {
	rows, err := s.q.ListRecordingsBySession(ctx, utils.UUIDToPg(sessionID))
	if err != nil {
		return nil, err
	}
	out := make([]Recording, 0, len(rows))
	for _, r := range rows {
		out = append(out, Recording{
			ID: utils.UUIDFromPg(r.ID), SessionID: sessionID.String(),
			FileKey: utils.TextFromPg(r.FileKey), Status: string(r.Status),
			DurationSec: utils.Int4FromPg(r.DurationSec), ThumbnailURL: utils.TextFromPg(r.ThumbnailUrl),
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

func (s *Service) ForInstructor(ctx context.Context, tenantID, instructorID uuid.UUID, limit, offset int32) ([]Recording, error) {
	rows, err := s.q.ListRecordingsForInstructor(ctx, db.ListRecordingsForInstructorParams{
		TenantID: utils.UUIDToPg(tenantID), InstructorID: utils.UUIDToPg(instructorID),
		Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Recording, 0, len(rows))
	for _, r := range rows {
		play, _ := s.storage.GetFileURL(utils.TextFromPg(r.FileKey))
		out = append(out, Recording{
			ID: utils.UUIDFromPg(r.ID), SessionID: utils.UUIDFromPg(r.SessionID),
			CourseID: utils.UUIDFromPg(r.CourseID), Title: r.SessionTitle,
			FileKey: utils.TextFromPg(r.FileKey), Status: string(r.Status),
			DurationSec: utils.Int4FromPg(r.DurationSec), ThumbnailURL: utils.TextFromPg(r.ThumbnailUrl),
			CreatedAt: r.CreatedAt.Time, PlayURL: play,
		})
	}
	return out, nil
}

// ForTenant is the student-facing "my recordings" list. TODO(Phase D): scope
// by entitlement/enrolment instead of the whole tenant.
func (s *Service) ForTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]Recording, error) {
	rows, err := s.q.ListRecordingsForTenant(ctx, db.ListRecordingsForTenantParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Recording, 0, len(rows))
	for _, r := range rows {
		out = append(out, Recording{
			ID: utils.UUIDFromPg(r.ID), SessionID: utils.UUIDFromPg(r.SessionID),
			CourseID: utils.UUIDFromPg(r.CourseID), Title: r.SessionTitle,
			FileKey: utils.TextFromPg(r.FileKey), Status: string(r.Status),
			DurationSec: utils.Int4FromPg(r.DurationSec), ThumbnailURL: utils.TextFromPg(r.ThumbnailUrl),
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// UploadRecordingFromFile — called by the RTMP "recording done" webhook.
func (s *Service) UploadRecordingFromFile(ctx context.Context, streamKey, filePath string) error {
	sess, err := s.q.GetLiveSessionByIngestKey(ctx, streamKey)
	if err != nil {
		return fmt.Errorf("session not found for key %s: %w", streamKey, err)
	}
	// The file is already on a shared volume; register it and let the
	// processing worker pull it into MinIO (Phase G). For now just record it.
	_, err = s.q.CreateRecording(ctx, db.CreateRecordingParams{
		TenantID:  sess.TenantID,
		SessionID: sess.ID,
		FileKey:   pgtype.Text{String: filePath, Valid: true},
		Status:    db.NullRecordingStatus{RecordingStatus: db.RecordingStatus("processing"), Valid: true},
	})
	return err
}
