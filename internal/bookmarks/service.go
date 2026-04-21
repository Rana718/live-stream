package bookmarks

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type CreateRequest struct {
	LectureID        *uuid.UUID `json:"lecture_id"`
	MaterialID       *uuid.UUID `json:"material_id"`
	TimestampSeconds int32      `json:"timestamp_seconds"`
	Note             string     `json:"note"`
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, req CreateRequest) (*db.Bookmark, error) {
	b, err := s.q.CreateBookmark(ctx, db.CreateBookmarkParams{
		UserID:           utils.UUIDToPg(userID),
		LectureID:        utils.UUIDPtrToPg(req.LectureID),
		MaterialID:       utils.UUIDPtrToPg(req.MaterialID),
		TimestampSeconds: utils.Int4ToPg(req.TimestampSeconds),
		Note:             utils.TextToPg(req.Note),
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Service) ListMine(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.ListMyBookmarksRow, error) {
	return s.q.ListMyBookmarks(ctx, db.ListMyBookmarksParams{
		UserID: utils.UUIDToPg(userID), Limit: limit, Offset: offset,
	})
}

func (s *Service) ListForLecture(ctx context.Context, userID, lectureID uuid.UUID) ([]db.Bookmark, error) {
	return s.q.ListMyBookmarksForLecture(ctx, db.ListMyBookmarksForLectureParams{
		UserID:    utils.UUIDToPg(userID),
		LectureID: utils.UUIDToPg(lectureID),
	})
}

func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.q.DeleteBookmark(ctx, db.DeleteBookmarkParams{
		ID: utils.UUIDToPg(id), UserID: utils.UUIDToPg(userID),
	})
}
