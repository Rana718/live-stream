package topics

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type UpsertTopicRequest struct {
	ChapterID    uuid.UUID `json:"chapter_id" validate:"required"`
	Name         string    `json:"name" validate:"required"`
	Description  string    `json:"description"`
	DisplayOrder int32     `json:"display_order"`
	IsFree       bool      `json:"is_free"`
}

func (s *Service) Create(ctx context.Context, req UpsertTopicRequest) (*db.Topic, error) {
	t, err := s.q.CreateTopic(ctx, db.CreateTopicParams{
		ChapterID:    utils.UUIDToPg(req.ChapterID),
		Name:         req.Name,
		Description:  utils.TextToPg(req.Description),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
		IsFree:       utils.BoolToPg(req.IsFree),
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*db.Topic, error) {
	t, err := s.q.GetTopicByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Service) ListByChapter(ctx context.Context, chapterID uuid.UUID) ([]db.Topic, error) {
	return s.q.ListTopicsByChapter(ctx, utils.UUIDToPg(chapterID))
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpsertTopicRequest) (*db.Topic, error) {
	t, err := s.q.UpdateTopic(ctx, db.UpdateTopicParams{
		ID:           utils.UUIDToPg(id),
		Name:         req.Name,
		Description:  utils.TextToPg(req.Description),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
		IsFree:       utils.BoolToPg(req.IsFree),
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteTopic(ctx, utils.UUIDToPg(id))
}
