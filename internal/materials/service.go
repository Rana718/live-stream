package materials

import (
	"context"
	"fmt"
	"io"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
)

type Service struct {
	q      *db.Queries
	minio  *minio.Client
	bucket string
}

func NewService(pool *pgxpool.Pool, mc *minio.Client, bucket string) *Service {
	return &Service{q: db.New(pool), minio: mc, bucket: bucket}
}

type UploadRequest struct {
	TopicID      *uuid.UUID `json:"topic_id"`
	ChapterID    *uuid.UUID `json:"chapter_id"`
	SubjectID    *uuid.UUID `json:"subject_id"`
	Title        string     `json:"title" validate:"required"`
	Description  string     `json:"description"`
	MaterialType string     `json:"material_type"`
	Language     string     `json:"language"`
	IsFree       bool       `json:"is_free"`
}

func (s *Service) Upload(ctx context.Context, uploader uuid.UUID, req UploadRequest, filename string, size int64, reader io.Reader, contentType string) (*db.StudyMaterial, error) {
	if req.MaterialType == "" {
		req.MaterialType = "pdf"
	}
	if req.Language == "" {
		req.Language = "en"
	}
	objectName := fmt.Sprintf("%s/%d-%s", req.MaterialType, time.Now().UnixNano(), filename)

	_, err := s.minio.PutObject(ctx, s.bucket, objectName, reader, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return nil, err
	}

	m, err := s.q.CreateStudyMaterial(ctx, db.CreateStudyMaterialParams{
		TopicID:      utils.UUIDPtrToPg(req.TopicID),
		ChapterID:    utils.UUIDPtrToPg(req.ChapterID),
		SubjectID:    utils.UUIDPtrToPg(req.SubjectID),
		Title:        req.Title,
		Description:  utils.TextToPg(req.Description),
		MaterialType: utils.TextToPg(req.MaterialType),
		FilePath:     objectName,
		FileSize:     utils.Int8ToPg(size),
		Language:     utils.TextToPg(req.Language),
		IsFree:       utils.BoolToPg(req.IsFree),
		UploadedBy:   utils.UUIDToPg(uploader),
	})
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*db.StudyMaterial, error) {
	m, err := s.q.GetStudyMaterialByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Service) ListByChapter(ctx context.Context, chapterID uuid.UUID) ([]db.StudyMaterial, error) {
	return s.q.ListMaterialsByChapter(ctx, utils.UUIDToPg(chapterID))
}

func (s *Service) ListByTopic(ctx context.Context, topicID uuid.UUID) ([]db.StudyMaterial, error) {
	return s.q.ListMaterialsByTopic(ctx, utils.UUIDToPg(topicID))
}

func (s *Service) ListBySubject(ctx context.Context, subjectID uuid.UUID) ([]db.StudyMaterial, error) {
	return s.q.ListMaterialsBySubject(ctx, utils.UUIDToPg(subjectID))
}

func (s *Service) GetDownloadURL(ctx context.Context, id uuid.UUID, ttl time.Duration) (string, error) {
	m, err := s.q.GetStudyMaterialByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return "", err
	}
	u, err := s.minio.PresignedGetObject(ctx, s.bucket, m.FilePath, ttl, nil)
	if err != nil {
		return "", err
	}
	_ = s.q.IncrementMaterialDownload(ctx, utils.UUIDToPg(id))
	return u.String(), nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	m, err := s.q.GetStudyMaterialByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return err
	}
	_ = s.minio.RemoveObject(ctx, s.bucket, m.FilePath, minio.RemoveObjectOptions{})
	return s.q.DeleteStudyMaterial(ctx, utils.UUIDToPg(id))
}
