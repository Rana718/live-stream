package recording

import (
	"context"
	"fmt"
	"io"
	"live-platform/internal/database/db"
	"live-platform/internal/events"
	"live-platform/internal/storage"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	queries  *db.Queries
	storage  *storage.MinIOClient
	producer *events.Producer
	pool     *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool, storage *storage.MinIOClient, producer *events.Producer) *Service {
	return &Service{
		queries:  db.New(pool),
		storage:  storage,
		producer: producer,
		pool:     pool,
	}
}

func (s *Service) UploadRecording(ctx context.Context, streamID uuid.UUID, filename string, reader io.Reader, size int64) (*db.Recording, error) {
	// Upload to MinIO
	err := s.storage.UploadFile(ctx, filename, reader, size, "video/webm")
	if err != nil {
		return nil, err
	}

	// Create recording in database
	recording, err := s.queries.CreateRecording(ctx, db.CreateRecordingParams{
		StreamID: pgtype.UUID{Bytes: streamID, Valid: true},
		FilePath: filename,
		Status:   pgtype.Text{String: "ready", Valid: true},
	})
	if err != nil {
		return nil, err
	}

	// Update with file size
	recording, err = s.queries.UpdateRecordingDetails(ctx, db.UpdateRecordingDetailsParams{
		ID:           recording.ID,
		FileSize:     pgtype.Int8{Int64: size, Valid: true},
		Duration:     pgtype.Int4{Int32: 0, Valid: false},
		ThumbnailUrl: pgtype.Text{String: "", Valid: false},
		Status:       pgtype.Text{String: "ready", Valid: true},
	})
	if err != nil {
		return nil, err
	}

	s.producer.PublishEvent(ctx, uuid.UUID(recording.ID.Bytes).String(), map[string]interface{}{
		"event":        "recording_uploaded",
		"recording_id": uuid.UUID(recording.ID.Bytes).String(),
		"stream_id":    streamID.String(),
		"file_size":    size,
		"timestamp":    time.Now(),
	})

	return &recording, nil
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

// GetRecordingsByInstructor returns all recordings for streams owned by an instructor
func (s *Service) GetRecordingsByInstructor(ctx context.Context, instructorID uuid.UUID) ([]map[string]interface{}, error) {
	// Query to get recordings with stream info for this instructor
	query := `
		SELECT 
			r.id, r.stream_id, r.file_path, r.status, r.duration, r.file_size, r.created_at,
			s.title as stream_title, s.stream_key
		FROM recordings r
		JOIN streams s ON r.stream_id = s.id
		WHERE s.instructor_id = $1
		ORDER BY r.created_at DESC
	`

	rows, err := s.pool.Query(ctx, query, instructorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id, streamID pgtype.UUID
		var filePath, status, streamTitle, streamKey string
		var duration pgtype.Int4
		var fileSize pgtype.Int8
		var createdAt pgtype.Timestamp

		err := rows.Scan(&id, &streamID, &filePath, &status, &duration, &fileSize, &createdAt, &streamTitle, &streamKey)
		if err != nil {
			continue
		}

		// Get presigned URL for playback
		playURL, _ := s.storage.GetFileURL(filePath)

		results = append(results, map[string]interface{}{
			"id":           uuid.UUID(id.Bytes).String(),
			"stream_id":    uuid.UUID(streamID.Bytes).String(),
			"stream_title": streamTitle,
			"file_path":    filePath,
			"status":       status,
			"duration":     duration.Int32,
			"file_size":    fileSize.Int64,
			"created_at":   createdAt.Time,
			"play_url":     playURL,
		})
	}

	return results, nil
}

// UploadRecordingFromFile uploads a recording file from nginx-rtmp to MinIO
func (s *Service) UploadRecordingFromFile(ctx context.Context, streamKey string, filePath string) error {
	log.Printf("UploadRecordingFromFile: streamKey=%s, filePath=%s", streamKey, filePath)

	// Get stream by key
	stream, err := s.queries.GetStreamByKey(ctx, streamKey)
	if err != nil {
		return fmt.Errorf("stream not found for key %s: %w", streamKey, err)
	}

	streamID := uuid.UUID(stream.ID.Bytes)

	// The recordings are in ./docker/recordings folder (shared volume)
	filename := filepath.Base(filePath)
	localPath := filepath.Join("docker", "recordings", filename)

	log.Printf("UploadRecordingFromFile: Reading from local path %s", localPath)

	// Open the local file
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open recording file %s: %w", localPath, err)
	}
	defer file.Close()

	// Get file info for size
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Upload to MinIO
	minioFilename := fmt.Sprintf("recordings/%s/%s", streamKey, filename)
	contentType := "video/x-flv"

	log.Printf("UploadRecordingFromFile: Uploading to MinIO as %s (size: %d bytes)", minioFilename, fileInfo.Size())

	err = s.storage.UploadFile(ctx, minioFilename, file, fileInfo.Size(), contentType)
	if err != nil {
		return fmt.Errorf("failed to upload to MinIO: %w", err)
	}

	// Create recording in database
	recording, err := s.queries.CreateRecording(ctx, db.CreateRecordingParams{
		StreamID: pgtype.UUID{Bytes: streamID, Valid: true},
		FilePath: minioFilename,
		Status:   pgtype.Text{String: "ready", Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to create recording in DB: %w", err)
	}

	// Update with file size
	_, err = s.queries.UpdateRecordingDetails(ctx, db.UpdateRecordingDetailsParams{
		ID:           recording.ID,
		FileSize:     pgtype.Int8{Int64: fileInfo.Size(), Valid: true},
		Duration:     pgtype.Int4{Int32: 0, Valid: false},
		ThumbnailUrl: pgtype.Text{String: "", Valid: false},
		Status:       pgtype.Text{String: "ready", Valid: true},
	})
	if err != nil {
		log.Printf("Warning: failed to update recording details: %v", err)
	}

	log.Printf("UploadRecordingFromFile: Recording saved - ID: %s, Stream: %s",
		uuid.UUID(recording.ID.Bytes).String(), streamID.String())

	// Publish event
	s.producer.PublishEvent(ctx, uuid.UUID(recording.ID.Bytes).String(), map[string]interface{}{
		"event":        "recording_uploaded",
		"recording_id": uuid.UUID(recording.ID.Bytes).String(),
		"stream_id":    streamID.String(),
		"timestamp":    time.Now(),
	})

	// Delete local file after successful upload to MinIO to avoid duplication
	if err := os.Remove(localPath); err != nil {
		log.Printf("Warning: failed to delete local recording file %s: %v", localPath, err)
	} else {
		log.Printf("UploadRecordingFromFile: Deleted local file %s after MinIO upload", localPath)
	}

	return nil
}
