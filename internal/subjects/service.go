package subjects

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type UpsertSubjectRequest struct {
	CourseID     uuid.UUID `json:"course_id" validate:"required"`
	Name         string    `json:"name" validate:"required"`
	Description  string    `json:"description"`
	IconURL      string    `json:"icon_url"`
	DisplayOrder int32     `json:"display_order"`
}

func (s *Service) Create(ctx context.Context, req UpsertSubjectRequest) (*db.Subject, error) {
	sub, err := s.q.CreateSubject(ctx, db.CreateSubjectParams{
		CourseID:     utils.UUIDToPg(req.CourseID),
		Name:         req.Name,
		Description:  utils.TextToPg(req.Description),
		IconUrl:      utils.TextToPg(req.IconURL),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
	})
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*db.Subject, error) {
	sub, err := s.q.GetSubjectByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) ListByCourse(ctx context.Context, courseID uuid.UUID) ([]db.Subject, error) {
	return s.q.ListSubjectsByCourse(ctx, utils.UUIDToPg(courseID))
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpsertSubjectRequest) (*db.Subject, error) {
	sub, err := s.q.UpdateSubject(ctx, db.UpdateSubjectParams{
		ID:           utils.UUIDToPg(id),
		Name:         req.Name,
		Description:  utils.TextToPg(req.Description),
		IconUrl:      utils.TextToPg(req.IconURL),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
	})
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteSubject(ctx, utils.UUIDToPg(id))
}
