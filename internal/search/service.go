// Package search — schema-v2. Course full-text search only (lectures are no
// longer a standalone searchable table; lesson search returns in Phase J
// with a content-search index).
package search

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

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type UnifiedResult struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Snippet     string `json:"snippet,omitempty"`
	Thumbnail   string `json:"thumbnail_url,omitempty"`
}

func (s *Service) Unified(ctx context.Context, tenantID uuid.UUID, q string, limit, offset int32) ([]UnifiedResult, error) {
	if tenantID == uuid.Nil || q == "" {
		return []UnifiedResult{}, nil
	}
	courses, err := s.q.SearchCourses(ctx, db.SearchCoursesParams{
		TenantID: utils.UUIDToPg(tenantID), Q: q, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]UnifiedResult, 0, len(courses))
	for _, c := range courses {
		summary := utils.TextFromPg(c.Summary)
		out = append(out, UnifiedResult{
			Type: "course", ID: utils.UUIDFromPg(c.ID), Title: c.Title,
			Description: summary, Snippet: summary, Thumbnail: utils.TextFromPg(c.ThumbnailUrl),
		})
	}
	return out, nil
}
