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

func (s *Service) Create(ctx context.Context, req UpsertChapterRequest) (*db.Chapter, error) {
	ch, err := s.q.CreateChapter(ctx, db.CreateChapterParams{
		SubjectID:    utils.UUIDToPg(req.SubjectID),
		Name:         req.Name,
		Description:  utils.TextToPg(req.Description),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
		IsFree:       utils.BoolToPg(req.IsFree),
	})
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*db.Chapter, error) {
	ch, err := s.q.GetChapterByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

func (s *Service) ListBySubject(ctx context.Context, subjectID uuid.UUID) ([]db.Chapter, error) {
	return s.q.ListChaptersBySubject(ctx, utils.UUIDToPg(subjectID))
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpsertChapterRequest) (*db.Chapter, error) {
	ch, err := s.q.UpdateChapter(ctx, db.UpdateChapterParams{
		ID:           utils.UUIDToPg(id),
		Name:         req.Name,
		Description:  utils.TextToPg(req.Description),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
		IsFree:       utils.BoolToPg(req.IsFree),
	})
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteChapter(ctx, utils.UUIDToPg(id))
}
