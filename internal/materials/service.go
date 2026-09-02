// Package materials — schema-v2 adapter over content_documents. The old
// `study_materials` table (topic/chapter/subject-scoped) is gone; documents
// now live in content_documents and are attached to courses via
// course_lessons. Per-chapter/topic listing returns empty until the Phase-J
// content UI; upload / get / presigned-download still work.
package materials

import (
	"context"
	"fmt"
	"io"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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
	Title        string `json:"title" validate:"required"`
	Description  string `json:"description"`
	MaterialType string `json:"material_type"`
}

type Material struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	FileKey  string `json:"file_key"`
	FilePath string `json:"file_path"`
	FileSize int64  `json:"file_size"`
	Mime     string `json:"mime"`
	FileType string `json:"file_type"`
}

func (s *Service) Upload(ctx context.Context, tenantID, uploader uuid.UUID, req UploadRequest, filename string, size int64, reader io.Reader, contentType string) (Material, error) {
	object := fmt.Sprintf("documents/%d-%s", time.Now().UnixNano(), filename)
	if _, err := s.minio.PutObject(ctx, s.bucket, object, reader, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return Material{}, err
	}
	d, err := s.q.CreateContentDocument(ctx, db.CreateContentDocumentParams{
		TenantID: utils.UUIDToPg(tenantID),
		Title:    req.Title,
		FileKey:  object,
		FileSize: pgtype.Int8{Int64: size, Valid: true},
		Mime:     pgtype.Text{String: contentType, Valid: contentType != ""},
	})
	if err != nil {
		return Material{}, err
	}
	return Material{
		ID: utils.UUIDFromPg(d.ID), Title: d.Title, FileKey: d.FileKey, FilePath: d.FileKey,
		FileSize: d.FileSize, Mime: utils.TextFromPg(d.Mime), FileType: utils.TextFromPg(d.Mime),
	}, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Material, error) {
	d, err := s.q.GetContentDocument(ctx, utils.UUIDToPg(id))
	if err != nil {
		return Material{}, err
	}
	return Material{
		ID: utils.UUIDFromPg(d.ID), Title: d.Title, FileKey: d.FileKey, FilePath: d.FileKey,
		FileSize: d.FileSize, Mime: utils.TextFromPg(d.Mime), FileType: utils.TextFromPg(d.Mime),
	}, nil
}

func (s *Service) ListForTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]Material, error) {
	rows, err := s.q.ListContentDocumentsForTenant(ctx, db.ListContentDocumentsForTenantParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Material, 0, len(rows))
	for _, r := range rows {
		out = append(out, Material{
			ID: utils.UUIDFromPg(r.ID), Title: r.Title, FileKey: r.FileKey, FilePath: r.FileKey,
			FileSize: r.FileSize, Mime: utils.TextFromPg(r.Mime), FileType: utils.TextFromPg(r.Mime),
		})
	}
	return out, nil
}

func (s *Service) GetDownloadURL(ctx context.Context, id uuid.UUID, ttl time.Duration) (string, error) {
	d, err := s.q.GetContentDocument(ctx, utils.UUIDToPg(id))
	if err != nil {
		return "", err
	}
	u, err := s.minio.PresignedGetObject(ctx, s.bucket, d.FileKey, ttl, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *Service) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	d, err := s.q.GetContentDocument(ctx, utils.UUIDToPg(id))
	if err != nil {
		return err
	}
	_ = s.minio.RemoveObject(ctx, s.bucket, d.FileKey, minio.RemoveObjectOptions{})
	return s.q.DeleteContentDocument(ctx, db.DeleteContentDocumentParams{
		ID: utils.UUIDToPg(id), TenantID: utils.UUIDToPg(tenantID),
	})
}
