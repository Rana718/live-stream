package search

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service aggregates searches across content types (courses, lectures, PYQs).
type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type UnifiedResult struct {
	Type        string `json:"type"` // "course" | "lecture"
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Thumbnail   string `json:"thumbnail_url,omitempty"`
	Language    string `json:"language,omitempty"`
}

func (s *Service) Unified(ctx context.Context, q string, limit, offset int32) ([]UnifiedResult, error) {
	courses, err := s.q.SearchCourses(ctx, db.SearchCoursesParams{
		PlaintoTsquery: q, Column2: q, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	lectures, err := s.q.SearchLectures(ctx, db.SearchLecturesParams{
		PlaintoTsquery: q, Column2: q, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}

	out := make([]UnifiedResult, 0, len(courses)+len(lectures))
	for _, c := range courses {
		out = append(out, UnifiedResult{
			Type:        "course",
			ID:          utils.UUIDFromPg(c.ID),
			Title:       c.Title,
			Description: utils.TextFromPg(c.Description),
			Thumbnail:   utils.TextFromPg(c.ThumbnailUrl),
			Language:    utils.TextFromPg(c.Language),
		})
	}
	for _, l := range lectures {
		out = append(out, UnifiedResult{
			Type:        "lecture",
			ID:          utils.UUIDFromPg(l.ID),
			Title:       l.Title,
			Description: utils.TextFromPg(l.Description),
			Thumbnail:   utils.TextFromPg(l.ThumbnailUrl),
			Language:    utils.TextFromPg(l.Language),
		})
	}
	return out, nil
}
