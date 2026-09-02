package chapters

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type UpsertChapterRequest struct {
	SubjectID    uuid.UUID `json:"subject_id" validate:"required"`
	Name         string    `json:"name" validate:"required"`
	Description  string    `json:"description"`
	DisplayOrder int32     `json:"display_order"`
	IsFree       bool      `json:"is_free"`
}

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, req UpsertChapterRequest) (db.CreateChapterRow, error) {
	return s.q.CreateChapter(ctx, db.CreateChapterParams{
		TenantID:     utils.UUIDToPg(tenantID),
		SubjectID:    utils.UUIDToPg(req.SubjectID),
		Name:         req.Name,
		Description:  utils.TextToPg(req.Description),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
		IsFree:       utils.BoolToPg(req.IsFree),
	})
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.GetChapterRow, error) {
	return s.q.GetChapter(ctx, utils.UUIDToPg(id))
}

func (s *Service) ListBySubject(ctx context.Context, tenantID, subjectID uuid.UUID) ([]db.ListChaptersBySubjectRow, error) {
	return s.q.ListChaptersBySubject(ctx, db.ListChaptersBySubjectParams{
		TenantID:  utils.UUIDToPg(tenantID),
		SubjectID: utils.UUIDToPg(subjectID),
	})
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpsertChapterRequest) (db.UpdateChapterRow, error) {
	return s.q.UpdateChapter(ctx, db.UpdateChapterParams{
		ID:           utils.UUIDToPg(id),
		Name:         utils.TextToPg(req.Name),
		Description:  utils.TextToPg(req.Description),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
		IsFree:       utils.BoolToPg(req.IsFree),
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteChapter(ctx, utils.UUIDToPg(id))
}
