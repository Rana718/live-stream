package downloads

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
)

type Service struct {
	q        *db.Queries
	minio    *minio.Client
	bucket   string
	baseURL  string
}

func NewService(pool *pgxpool.Pool, mc *minio.Client, bucket, baseURL string) *Service {
	return &Service{q: db.New(pool), minio: mc, bucket: bucket, baseURL: baseURL}
}

// --- Video variants ---

type CreateVariantRequest struct {
	RecordingID *uuid.UUID `json:"recording_id"`
	LectureID   *uuid.UUID `json:"lecture_id"`
	Quality     string     `json:"quality" validate:"required,oneof=240p 360p 480p 720p 1080p"`
	FilePath    string     `json:"file_path" validate:"required"`
	FileSize    int64      `json:"file_size"`
	BitrateKbps int32      `json:"bitrate_kbps"`
	Width       int32      `json:"width"`
	Height      int32      `json:"height"`
	Codec       string     `json:"codec"`
}

func (s *Service) CreateVariant(ctx context.Context, req CreateVariantRequest) (*db.VideoVariant, error) {
	if req.Codec == "" {
		req.Codec = "h264"
	}
	v, err := s.q.CreateVideoVariant(ctx, db.CreateVideoVariantParams{
		RecordingID: utils.UUIDPtrToPg(req.RecordingID),
		LectureID:   utils.UUIDPtrToPg(req.LectureID),
		Quality:     req.Quality,
		FilePath:    req.FilePath,
		FileSize:    utils.Int8ToPg(req.FileSize),
		BitrateKbps: utils.Int4ToPg(req.BitrateKbps),
		Width:       utils.Int4ToPg(req.Width),
		Height:      utils.Int4ToPg(req.Height),
		Codec:       utils.TextToPg(req.Codec),
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Service) ListVariantsForLecture(ctx context.Context, lectureID uuid.UUID) ([]db.VideoVariant, error) {
	return s.q.ListVariantsByLecture(ctx, utils.UUIDPtrToPg(&lectureID))
}

func (s *Service) ListVariantsForRecording(ctx context.Context, recordingID uuid.UUID) ([]db.VideoVariant, error) {
	return s.q.ListVariantsByRecording(ctx, utils.UUIDPtrToPg(&recordingID))
}

// --- Download tokens (time-limited access for offline use) ---

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type TokenRequest struct {
	ResourceType string    `json:"resource_type" validate:"required,oneof=lecture recording material variant"`
	ResourceID   uuid.UUID `json:"resource_id" validate:"required"`
	TTLSeconds   int32     `json:"ttl_seconds"`
}

type TokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	URL       string    `json:"url"`
}

func (s *Service) IssueToken(ctx context.Context, userID uuid.UUID, req TokenRequest) (*TokenResponse, error) {
	if req.TTLSeconds <= 0 {
		req.TTLSeconds = 3600 // 1 hour default
	}
	token, err := generateToken()
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(time.Duration(req.TTLSeconds) * time.Second)
	_, err = s.q.CreateDownloadToken(ctx, db.CreateDownloadTokenParams{
		UserID:       utils.UUIDToPg(userID),
		ResourceType: req.ResourceType,
		ResourceID:   utils.UUIDToPg(req.ResourceID),
		Token:        token,
		ExpiresAt:    utils.TimestampToPg(expires),
	})
	if err != nil {
		return nil, err
	}
	return &TokenResponse{
		Token:     token,
		ExpiresAt: expires,
		URL:       s.baseURL + "/api/v1/downloads/fetch?token=" + token,
	}, nil
}

// Resolve validates a download token and returns a presigned URL to the file.
func (s *Service) Resolve(ctx context.Context, tokenStr string) (string, error) {
	t, err := s.q.GetDownloadTokenByToken(ctx, tokenStr)
	if err != nil {
		return "", errors.New("invalid or expired token")
	}
	var key string
	switch t.ResourceType {
	case "variant":
		v, err := s.q.GetVideoVariantByID(ctx, t.ResourceID)
		if err != nil {
			return "", err
		}
		key = v.FilePath
	case "material":
		m, err := s.q.GetStudyMaterialByID(ctx, t.ResourceID)
		if err != nil {
			return "", err
		}
		key = m.FilePath
	case "recording":
		r, err := s.q.GetRecordingByID(ctx, t.ResourceID)
		if err != nil {
			return "", err
		}
		key = r.FilePath
	default:
		return "", errors.New("unsupported resource type")
	}

	u, err := s.minio.PresignedGetObject(ctx, s.bucket, key, 15*time.Minute, nil)
	if err != nil {
		return "", err
	}
	_ = s.q.MarkDownloadTokenUsed(ctx, t.ID)
	return u.String(), nil
}

func (s *Service) PurgeExpired(ctx context.Context) error {
	return s.q.PurgeExpiredDownloadTokens(ctx)
}
