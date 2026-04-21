package exams

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

type UpsertCategoryRequest struct {
	Name         string `json:"name" validate:"required,min=2"`
	Slug         string `json:"slug" validate:"required,min=2"`
	Description  string `json:"description"`
	IconURL      string `json:"icon_url"`
	DisplayOrder int32  `json:"display_order"`
	IsActive     bool   `json:"is_active"`
}

func (s *Service) Create(ctx context.Context, req UpsertCategoryRequest) (*db.ExamCategory, error) {
	c, err := s.q.CreateExamCategory(ctx, db.CreateExamCategoryParams{
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  utils.TextToPg(req.Description),
		IconUrl:      utils.TextToPg(req.IconURL),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*db.ExamCategory, error) {
	c, err := s.q.GetExamCategoryByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) List(ctx context.Context) ([]db.ExamCategory, error) {
	return s.q.ListExamCategories(ctx)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpsertCategoryRequest) (*db.ExamCategory, error) {
	c, err := s.q.UpdateExamCategory(ctx, db.UpdateExamCategoryParams{
		ID:           utils.UUIDToPg(id),
		Name:         req.Name,
		Description:  utils.TextToPg(req.Description),
		IconUrl:      utils.TextToPg(req.IconURL),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
		IsActive:     utils.BoolToPg(req.IsActive),
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteExamCategory(ctx, utils.UUIDToPg(id))
}
