package bookmarks

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// schema-v2: `lesson_bookmarks` replaces the old lecture/material bookmarks —
// keyed on course_lessons.id, with position_sec + note. user-owned.
type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type CreateRequest struct {
	LessonID    *uuid.UUID `json:"lesson_id"`
	LectureID   *uuid.UUID `json:"lecture_id"` // legacy alias
	PositionSec int32      `json:"position_sec"`
	Note        string     `json:"note"`
}

func (r CreateRequest) lesson() *uuid.UUID {
	if r.LessonID != nil {
		return r.LessonID
	}
	return r.LectureID
}

func (s *Service) Create(ctx context.Context, tenantID, userID uuid.UUID, req CreateRequest) (db.CreateLessonBookmarkRow, error) {
	return s.q.CreateLessonBookmark(ctx, db.CreateLessonBookmarkParams{
		TenantID:    utils.UUIDToPg(tenantID),
		UserID:      utils.UUIDToPg(userID),
		LessonID:    utils.UUIDPtrToPg(req.lesson()),
		PositionSec: utils.Int4ToPg(req.PositionSec),
		Note:        utils.TextToPg(req.Note),
	})
}

func (s *Service) ListMine(ctx context.Context, tenantID, userID uuid.UUID) ([]db.ListLessonBookmarksRow, error) {
	return s.q.ListLessonBookmarks(ctx, db.ListLessonBookmarksParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
}

func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.q.DeleteLessonBookmark(ctx, db.DeleteLessonBookmarkParams{
		ID: utils.UUIDToPg(id), UserID: utils.UUIDToPg(userID),
	})
}
