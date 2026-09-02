// Package cms backs the marketing site's blog, FAQ and free-form pages.
// Platform-wide (not tenant-scoped); reads are public, writes are
// super_admin-gated in the router. schema-v2.
package cms

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

func ntext(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func emptyJSONIfBlank(s string) string {
	if strings.TrimSpace(s) == "" {
		return "{}"
	}
	return s
}

func estimateMinutes(html string) int32 {
	words := len(strings.Fields(html))
	m := int32(words / 200)
	if m < 1 {
		m = 1
	}
	return m
}

// ── blog posts ──────────────────────────────────────────────────────

func (s *Service) ListPublishedPosts(ctx context.Context, limit, offset int32) ([]db.ListBlogPostsRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	return s.q.ListBlogPosts(ctx, db.ListBlogPostsParams{Limit: limit, Offset: offset})
}

func (s *Service) GetPostBySlug(ctx context.Context, slug string) (db.GetBlogPostRow, error) {
	return s.q.GetBlogPost(ctx, slug)
}

func (s *Service) AdminListPosts(ctx context.Context, limit, offset int32) ([]db.AdminListBlogPostsRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return s.q.AdminListBlogPosts(ctx, db.AdminListBlogPostsParams{Limit: limit, Offset: offset})
}

type PostInput struct {
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Excerpt     string     `json:"excerpt"`
	BodyJSON    string     `json:"body_json"`
	BodyHTML    string     `json:"body_html"`
	CoverURL    string     `json:"cover_url"`
	AuthorName  string     `json:"author_name"`
	Tags        []string   `json:"tags"`
	PublishedAt *time.Time `json:"published_at"`
	MinutesRead int32      `json:"minutes_read"`
	SeoTitle    string     `json:"seo_title"`
	SeoDesc     string     `json:"seo_desc"`
}

func (s *Service) upsertPost(ctx context.Context, in PostInput, byUser uuid.UUID) (db.UpsertBlogPostRow, error) {
	if strings.TrimSpace(in.Title) == "" {
		return db.UpsertBlogPostRow{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(in.Slug) == "" {
		in.Slug = slugify(in.Title)
	}
	if in.MinutesRead <= 0 {
		in.MinutesRead = estimateMinutes(in.BodyHTML)
	}
	pub := pgtype.Timestamptz{}
	if in.PublishedAt != nil {
		pub = pgtype.Timestamptz{Time: *in.PublishedAt, Valid: true}
	}
	return s.q.UpsertBlogPost(ctx, db.UpsertBlogPostParams{
		Slug:        in.Slug,
		Title:       in.Title,
		Excerpt:     ntext(in.Excerpt),
		BodyJson:    []byte(emptyJSONIfBlank(in.BodyJSON)),
		BodyHtml:    ntext(in.BodyHTML),
		CoverUrl:    ntext(in.CoverURL),
		AuthorName:  ntext(in.AuthorName),
		Tags:        in.Tags,
		MinutesRead: pgtype.Int4{Int32: in.MinutesRead, Valid: true},
		SeoTitle:    ntext(in.SeoTitle),
		SeoDesc:     ntext(in.SeoDesc),
		PublishedAt: pub,
		CreatedBy:   pgtype.UUID{Bytes: byUser, Valid: byUser != uuid.Nil},
	})
}

func (s *Service) CreatePost(ctx context.Context, in PostInput, byUser uuid.UUID) (db.UpsertBlogPostRow, error) {
	return s.upsertPost(ctx, in, byUser)
}

func (s *Service) UpdatePost(ctx context.Context, id uuid.UUID, in PostInput) (db.UpsertBlogPostRow, error) {
	existing, err := s.q.GetBlogPostByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return db.UpsertBlogPostRow{}, fmt.Errorf("post not found")
	}
	in.Slug = existing.Slug
	return s.upsertPost(ctx, in, uuid.Nil)
}

func (s *Service) DeletePost(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteBlogPost(ctx, utils.UUIDToPg(id))
}

// ── faqs ────────────────────────────────────────────────────────────

func (s *Service) ListFaqs(ctx context.Context, category string) ([]db.ListFaqsRow, error) {
	return s.q.ListFaqs(ctx, ntext(category))
}

func (s *Service) ListHomepageFaqs(ctx context.Context) ([]db.ListFaqsRow, error) {
	all, err := s.q.ListFaqs(ctx, pgtype.Text{})
	if err != nil {
		return nil, err
	}
	out := make([]db.ListFaqsRow, 0)
	for _, f := range all {
		if f.ShowOnHome {
			out = append(out, f)
		}
	}
	return out, nil
}

func (s *Service) AdminListFaqs(ctx context.Context) ([]db.AdminListFaqsRow, error) {
	return s.q.AdminListFaqs(ctx)
}

type FaqInput struct {
	Category     string `json:"category"`
	Question     string `json:"question"`
	AnswerHTML   string `json:"answer_html"`
	ShowOnHome   bool   `json:"show_on_home"`
	IsActive     *bool  `json:"is_active"`
	DisplayOrder int32  `json:"display_order"`
}

func (s *Service) CreateFaq(ctx context.Context, in FaqInput) (db.CreateFaqRow, error) {
	return s.q.CreateFaq(ctx, db.CreateFaqParams{
		Question:     in.Question,
		AnswerHtml:   in.AnswerHTML,
		Category:     ntext(in.Category),
		ShowOnHome:   pgtype.Bool{Bool: in.ShowOnHome, Valid: true},
		DisplayOrder: pgtype.Int4{Int32: in.DisplayOrder, Valid: true},
	})
}

func (s *Service) UpdateFaq(ctx context.Context, id uuid.UUID, in FaqInput) (db.UpdateFaqRow, error) {
	active := pgtype.Bool{}
	if in.IsActive != nil {
		active = pgtype.Bool{Bool: *in.IsActive, Valid: true}
	}
	return s.q.UpdateFaq(ctx, db.UpdateFaqParams{
		ID:           utils.UUIDToPg(id),
		Category:     ntext(in.Category),
		Question:     ntext(in.Question),
		AnswerHtml:   ntext(in.AnswerHTML),
		ShowOnHome:   pgtype.Bool{Bool: in.ShowOnHome, Valid: true},
		IsActive:     active,
		DisplayOrder: pgtype.Int4{Int32: in.DisplayOrder, Valid: in.DisplayOrder > 0},
	})
}

func (s *Service) DeleteFaq(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteFaq(ctx, utils.UUIDToPg(id))
}

// ── cms pages ───────────────────────────────────────────────────────

func (s *Service) GetCmsPage(ctx context.Context, slug string) (db.GetCmsPageRow, error) {
	return s.q.GetCmsPage(ctx, slug)
}

func (s *Service) AdminListCmsPages(ctx context.Context) ([]db.AdminListCmsPagesRow, error) {
	return s.q.AdminListCmsPages(ctx)
}

func (s *Service) AdminGetCmsPage(ctx context.Context, slug string) (db.AdminGetCmsPageRow, error) {
	return s.q.AdminGetCmsPage(ctx, slug)
}

type CmsPageInput struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	BodyJSON    string `json:"body_json"`
	BodyHTML    string `json:"body_html"`
	SeoTitle    string `json:"seo_title"`
	SeoDesc     string `json:"seo_desc"`
	IsPublished *bool  `json:"is_published"`
}

func (s *Service) UpsertCmsPage(ctx context.Context, in CmsPageInput) (db.AdminGetCmsPageRow, error) {
	pub := pgtype.Bool{Bool: true, Valid: true}
	if in.IsPublished != nil {
		pub = pgtype.Bool{Bool: *in.IsPublished, Valid: true}
	}
	if err := s.q.UpsertCmsPage(ctx, db.UpsertCmsPageParams{
		Slug:        in.Slug,
		Title:       in.Title,
		BodyJson:    []byte(emptyJSONIfBlank(in.BodyJSON)),
		BodyHtml:    ntext(in.BodyHTML),
		SeoTitle:    ntext(in.SeoTitle),
		SeoDesc:     ntext(in.SeoDesc),
		IsPublished: pub,
	}); err != nil {
		return db.AdminGetCmsPageRow{}, err
	}
	return s.q.AdminGetCmsPage(ctx, in.Slug)
}
