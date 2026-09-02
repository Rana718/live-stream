package subjects

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// schema-v2: subjects are tenant-scoped and no longer belong to a single
// course — they hang off the tenant (+ optional exam_category) and are
// reusable across courses. The /subjects/course/:id route now returns every
// subject in the tenant (course→subject scoping is a Phase-J follow-up).
type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type UpsertSubjectRequest struct {
	Name           string     `json:"name" validate:"required"`
	Code           string     `json:"code"`
	IconURL        string     `json:"icon_url"`
	ExamCategoryID *uuid.UUID `json:"exam_category_id"`
	DisplayOrder   int32      `json:"display_order"`
}

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, req UpsertSubjectRequest) (db.CreateSubjectRow, error) {
	return s.q.CreateSubject(ctx, db.CreateSubjectParams{
		TenantID:       utils.UUIDToPg(tenantID),
		Name:           req.Name,
		Code:           utils.TextToPg(req.Code),
		IconUrl:        utils.TextToPg(req.IconURL),
		ExamCategoryID: utils.UUIDPtrToPg(req.ExamCategoryID),
		DisplayOrder:   utils.Int4ToPg(req.DisplayOrder),
	})
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.GetSubjectRow, error) {
	return s.q.GetSubject(ctx, utils.UUIDToPg(id))
}

func (s *Service) ListForTenant(ctx context.Context, tenantID uuid.UUID) ([]db.ListSubjectsRow, error) {
	return s.q.ListSubjects(ctx, utils.UUIDToPg(tenantID))
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpsertSubjectRequest) (db.UpdateSubjectRow, error) {
	return s.q.UpdateSubject(ctx, db.UpdateSubjectParams{
		ID:             utils.UUIDToPg(id),
		Name:           utils.TextToPg(req.Name),
		Code:           utils.TextToPg(req.Code),
		IconUrl:        utils.TextToPg(req.IconURL),
		ExamCategoryID: utils.UUIDPtrToPg(req.ExamCategoryID),
		DisplayOrder:   utils.Int4ToPg(req.DisplayOrder),
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteSubject(ctx, utils.UUIDToPg(id))
}
