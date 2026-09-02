package exams

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// exam_categories are platform-level (no tenant). Public read; super_admin
// write. In schema-v2 they carry an optional self-referential parent_id
// (e.g. "JEE" › "JEE Main").
type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

type UpsertCategoryRequest struct {
	Name         string     `json:"name" validate:"required,min=2"`
	Slug         string     `json:"slug" validate:"required,min=2"`
	Description  string     `json:"description"`
	IconURL      string     `json:"icon_url"`
	ParentID     *uuid.UUID `json:"parent_id"`
	DisplayOrder int32      `json:"display_order"`
	IsActive     *bool      `json:"is_active"`
}

func (s *Service) Create(ctx context.Context, req UpsertCategoryRequest) (db.CreateExamCategoryRow, error) {
	return s.q.CreateExamCategory(ctx, db.CreateExamCategoryParams{
		Name:         req.Name,
		Slug:         req.Slug,
		ParentID:     utils.UUIDPtrToPg(req.ParentID),
		Description:  utils.TextToPg(req.Description),
		IconUrl:      utils.TextToPg(req.IconURL),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
	})
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.GetExamCategoryRow, error) {
	return s.q.GetExamCategory(ctx, utils.UUIDToPg(id))
}

func (s *Service) List(ctx context.Context) ([]db.ListExamCategoriesRow, error) {
	return s.q.ListExamCategories(ctx)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpsertCategoryRequest) (db.UpdateExamCategoryRow, error) {
	isActive := pgtype.Bool{}
	if req.IsActive != nil {
		isActive = pgtype.Bool{Bool: *req.IsActive, Valid: true}
	}
	return s.q.UpdateExamCategory(ctx, db.UpdateExamCategoryParams{
		ID:           utils.UUIDToPg(id),
		Name:         utils.TextToPg(req.Name),
		Description:  utils.TextToPg(req.Description),
		IconUrl:      utils.TextToPg(req.IconURL),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
		IsActive:     isActive,
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteExamCategory(ctx, utils.UUIDToPg(id))
}
