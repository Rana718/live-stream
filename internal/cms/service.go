// Package cms backs the marketing site's blog, FAQ, and free-form pages
// (terms / privacy / refund-policy / etc). The data is platform-wide —
// not tenant-scoped — so reads bypass tenant RLS and writes are gated on
// the super_admin role.
//
// Storage uses TipTap JSON for the canonical document plus a pre-rendered
// HTML cache. The admin UI edits TipTap; the API renders to HTML on save
// and stores both. Public read endpoints serve the HTML directly so the
// marketing site doesn't need to know about TipTap at all.
package cms

import (
	"context"
	"fmt"
	"strings"
	"time"

	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

// PostSummary is the index-page payload. Body is omitted to keep the
// blog index responses small.
type PostSummary struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Excerpt     string    `json:"excerpt"`
	CoverURL    string    `json:"cover_url,omitempty"`
	AuthorName  string    `json:"author_name,omitempty"`
	Tags        []string  `json:"tags"`
	PublishedAt time.Time `json:"published_at"`
	MinutesRead int32     `json:"minutes_read"`
}

// Post is the full read shape served on /blog/[slug].
type Post struct {
	PostSummary
	BodyHTML string `json:"body_html"`
	SeoTitle string `json:"seo_title,omitempty"`
	SeoDesc  string `json:"seo_desc,omitempty"`
}

func (s *Service) ListPublishedPosts(ctx context.Context, limit, offset int32) ([]PostSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.q.ListPublishedPosts(ctx, db.ListPublishedPostsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	out := make([]PostSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, PostSummary{
			ID:          uuid.UUID(r.ID.Bytes).String(),
			Slug:        r.Slug,
			Title:       r.Title,
			Excerpt:     r.Excerpt.String,
			CoverURL:    r.CoverUrl.String,
			AuthorName:  r.AuthorName.String,
			Tags:        r.Tags,
			PublishedAt: r.PublishedAt.Time,
			MinutesRead: r.MinutesRead,
		})
	}
	return out, nil
}

func (s *Service) GetPostBySlug(ctx context.Context, slug string) (*Post, error) {
	r, err := s.q.GetPostBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	return &Post{
		PostSummary: PostSummary{
			ID:          uuid.UUID(r.ID.Bytes).String(),
			Slug:        r.Slug,
			Title:       r.Title,
			Excerpt:     r.Excerpt.String,
			CoverURL:    r.CoverUrl.String,
			AuthorName:  r.AuthorName.String,
			Tags:        r.Tags,
			PublishedAt: r.PublishedAt.Time,
			MinutesRead: r.MinutesRead,
		},
		BodyHTML: r.BodyHtml,
		SeoTitle: r.SeoTitle.String,
		SeoDesc:  r.SeoDesc.String,
	}, nil
}

// PostInput is the admin create/update payload. body_json is the
// authoritative TipTap document; body_html is what the admin previewer
// shipped (saves the server an HTML render).
type PostInput struct {
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Excerpt     string    `json:"excerpt"`
	BodyJSON    string    `json:"body_json"`
	BodyHTML    string    `json:"body_html"`
	CoverURL    string    `json:"cover_url"`
	AuthorName  string    `json:"author_name"`
	Tags        []string  `json:"tags"`
	PublishedAt *time.Time `json:"published_at"`
	MinutesRead int32     `json:"minutes_read"`
	SeoTitle    string    `json:"seo_title"`
	SeoDesc     string    `json:"seo_desc"`
}

func (s *Service) CreatePost(ctx context.Context, in PostInput, byUser uuid.UUID) (*Post, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(in.Slug) == "" {
		in.Slug = slugify(in.Title)
	}
	if in.MinutesRead <= 0 {
		in.MinutesRead = estimateMinutes(in.BodyHTML)
	}
	pubAt := pgtype.Timestamptz{Valid: false}
	if in.PublishedAt != nil {
		pubAt = pgtype.Timestamptz{Time: *in.PublishedAt, Valid: true}
	}
	row, err := s.q.CreatePost(ctx, db.CreatePostParams{
		Slug:        in.Slug,
		Title:       in.Title,
		Excerpt:     pgtype.Text{String: in.Excerpt, Valid: in.Excerpt != ""},
		BodyJson:    []byte(emptyJSONIfBlank(in.BodyJSON)),
		BodyHtml:    in.BodyHTML,
		CoverUrl:    pgtype.Text{String: in.CoverURL, Valid: in.CoverURL != ""},
		AuthorName:  pgtype.Text{String: in.AuthorName, Valid: in.AuthorName != ""},
		Tags:        in.Tags,
		PublishedAt: pubAt,
		MinutesRead: in.MinutesRead,
		SeoTitle:    pgtype.Text{String: in.SeoTitle, Valid: in.SeoTitle != ""},
		SeoDesc:     pgtype.Text{String: in.SeoDesc, Valid: in.SeoDesc != ""},
		CreatedBy:   pgtype.UUID{Bytes: byUser, Valid: byUser != uuid.Nil},
	})
	if err != nil {
		return nil, err
	}
	return &Post{
		PostSummary: PostSummary{
			ID:          uuid.UUID(row.ID.Bytes).String(),
			Slug:        row.Slug,
			Title:       row.Title,
			Excerpt:     row.Excerpt.String,
			CoverURL:    row.CoverUrl.String,
			AuthorName:  row.AuthorName.String,
			Tags:        row.Tags,
			PublishedAt: row.PublishedAt.Time,
			MinutesRead: row.MinutesRead,
		},
		BodyHTML: row.BodyHtml,
	}, nil
}

func (s *Service) UpdatePost(ctx context.Context, id uuid.UUID, in PostInput) (*Post, error) {
	pubAt := pgtype.Timestamptz{Valid: false}
	if in.PublishedAt != nil {
		pubAt = pgtype.Timestamptz{Time: *in.PublishedAt, Valid: true}
	}
	row, err := s.q.UpdatePost(ctx, db.UpdatePostParams{
		ID:          pgtype.UUID{Bytes: id, Valid: true},
		Column2:     in.Title,
		Column3:     in.Excerpt,
		BodyJson:    []byte(emptyJSONIfBlank(in.BodyJSON)),
		Column5:     in.BodyHTML,
		Column6:     in.CoverURL,
		Column7:     in.AuthorName,
		Tags:        in.Tags,
		PublishedAt: pubAt,
		Column10:    in.MinutesRead,
		Column11:    in.SeoTitle,
		Column12:    in.SeoDesc,
	})
	if err != nil {
		return nil, err
	}
	return &Post{
		PostSummary: PostSummary{
			ID:          uuid.UUID(row.ID.Bytes).String(),
			Slug:        row.Slug,
			Title:       row.Title,
			Excerpt:     row.Excerpt.String,
			CoverURL:    row.CoverUrl.String,
			AuthorName:  row.AuthorName.String,
			Tags:        row.Tags,
			PublishedAt: row.PublishedAt.Time,
			MinutesRead: row.MinutesRead,
		},
		BodyHTML: row.BodyHtml,
	}, nil
}

func (s *Service) DeletePost(ctx context.Context, id uuid.UUID) error {
	return s.q.DeletePost(ctx, pgtype.UUID{Bytes: id, Valid: true})
}

func (s *Service) AdminListPosts(ctx context.Context, limit, offset int32) ([]PostSummary, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.q.AdminListPosts(ctx, db.AdminListPostsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	out := make([]PostSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, PostSummary{
			ID:          uuid.UUID(r.ID.Bytes).String(),
			Slug:        r.Slug,
			Title:       r.Title,
			Excerpt:     r.Excerpt.String,
			CoverURL:    r.CoverUrl.String,
			AuthorName:  r.AuthorName.String,
			Tags:        r.Tags,
			PublishedAt: r.PublishedAt.Time,
			MinutesRead: r.MinutesRead,
		})
	}
	return out, nil
}

// FAQ ----------------------------------------------------------------------

type FaqItem struct {
	ID           string `json:"id"`
	Category     string `json:"category"`
	Question     string `json:"question"`
	AnswerHTML   string `json:"answer_html"`
	ShowOnHome   bool   `json:"show_on_home"`
	IsActive     bool   `json:"is_active"`
	DisplayOrder int32  `json:"display_order"`
}

func (s *Service) ListFaqs(ctx context.Context, category string) ([]FaqItem, error) {
	rows, err := s.q.ListFaqs(ctx, category)
	if err != nil {
		return nil, err
	}
	out := make([]FaqItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, FaqItem{
			ID:           uuid.UUID(r.ID.Bytes).String(),
			Category:     r.Category,
			Question:     r.Question,
			AnswerHTML:   r.AnswerHtml,
			ShowOnHome:   r.ShowOnHome,
			IsActive:     true,
			DisplayOrder: r.DisplayOrder,
		})
	}
	return out, nil
}

func (s *Service) ListHomepageFaqs(ctx context.Context) ([]FaqItem, error) {
	rows, err := s.q.ListHomepageFaqs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FaqItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, FaqItem{
			ID:           uuid.UUID(r.ID.Bytes).String(),
			Category:     r.Category,
			Question:     r.Question,
			AnswerHTML:   r.AnswerHtml,
			ShowOnHome:   true,
			IsActive:     true,
			DisplayOrder: r.DisplayOrder,
		})
	}
	return out, nil
}

func (s *Service) AdminListFaqs(ctx context.Context) ([]FaqItem, error) {
	rows, err := s.q.AdminListFaqs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FaqItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, FaqItem{
			ID:           uuid.UUID(r.ID.Bytes).String(),
			Category:     r.Category,
			Question:     r.Question,
			AnswerHTML:   r.AnswerHtml,
			ShowOnHome:   r.ShowOnHome,
			IsActive:     r.IsActive,
			DisplayOrder: r.DisplayOrder,
		})
	}
	return out, nil
}

type FaqInput struct {
	Category     string `json:"category"`
	Question     string `json:"question"`
	AnswerHTML   string `json:"answer_html"`
	ShowOnHome   *bool  `json:"show_on_home"`
	IsActive     *bool  `json:"is_active"`
	DisplayOrder int32  `json:"display_order"`
}

func (s *Service) CreateFaq(ctx context.Context, in FaqInput) (*db.Faq, error) {
	if in.Category == "" {
		in.Category = "general"
	}
	if strings.TrimSpace(in.Question) == "" || strings.TrimSpace(in.AnswerHTML) == "" {
		return nil, fmt.Errorf("question and answer are required")
	}
	row, err := s.q.CreateFaq(ctx, db.CreateFaqParams{
		Category:    in.Category,
		Question:    in.Question,
		AnswerHtml:  in.AnswerHTML,
		Column4:     boolPtrToBool(in.ShowOnHome, false),
		Column5:     in.DisplayOrder,
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) UpdateFaq(ctx context.Context, id uuid.UUID, in FaqInput) (*db.Faq, error) {
	// `show_on_home` and `is_active` are non-null bool columns; sqlc
	// generated plain bool params, so a partial update isn't possible
	// for these fields without a SQL refactor. We default to the
	// existing value when the caller omitted the flag.
	cur, err := s.q.AdminListFaqs(ctx)
	if err != nil {
		return nil, err
	}
	var existing *db.Faq
	for i := range cur {
		if cur[i].ID == (pgtype.UUID{Bytes: id, Valid: true}) {
			existing = &cur[i]
			break
		}
	}
	showHome := boolPtrToBool(in.ShowOnHome, existing != nil && existing.ShowOnHome)
	isActive := boolPtrToBool(in.IsActive, existing != nil && existing.IsActive)
	row, err := s.q.UpdateFaq(ctx, db.UpdateFaqParams{
		ID:         pgtype.UUID{Bytes: id, Valid: true},
		Column2:    in.Category,
		Column3:    in.Question,
		Column4:    in.AnswerHTML,
		ShowOnHome: showHome,
		IsActive:   isActive,
		Column7:    in.DisplayOrder,
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) DeleteFaq(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteFaq(ctx, pgtype.UUID{Bytes: id, Valid: true})
}

// CMS pages ----------------------------------------------------------------

type CmsPage struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	BodyHTML    string `json:"body_html"`
	BodyJSON    string `json:"body_json,omitempty"`
	SeoTitle    string `json:"seo_title,omitempty"`
	SeoDesc     string `json:"seo_desc,omitempty"`
	IsPublished bool   `json:"is_published"`
}

func (s *Service) GetCmsPage(ctx context.Context, slug string) (*CmsPage, error) {
	r, err := s.q.GetCmsPage(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	return &CmsPage{
		Slug:        r.Slug,
		Title:       r.Title,
		BodyHTML:    r.BodyHtml,
		SeoTitle:    r.SeoTitle.String,
		SeoDesc:     r.SeoDesc.String,
		IsPublished: r.IsPublished,
	}, nil
}

func (s *Service) AdminGetCmsPage(ctx context.Context, slug string) (*CmsPage, error) {
	r, err := s.q.AdminGetCmsPage(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	return &CmsPage{
		Slug:        r.Slug,
		Title:       r.Title,
		BodyHTML:    r.BodyHtml,
		BodyJSON:    string(r.BodyJson),
		SeoTitle:    r.SeoTitle.String,
		SeoDesc:     r.SeoDesc.String,
		IsPublished: r.IsPublished,
	}, nil
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

func (s *Service) UpsertCmsPage(ctx context.Context, in CmsPageInput) (*CmsPage, error) {
	if strings.TrimSpace(in.Slug) == "" || strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("slug and title are required")
	}
	r, err := s.q.UpsertCmsPage(ctx, db.UpsertCmsPageParams{
		Slug:     in.Slug,
		Title:    in.Title,
		BodyJson: []byte(emptyJSONIfBlank(in.BodyJSON)),
		BodyHtml: in.BodyHTML,
		SeoTitle: pgtype.Text{String: in.SeoTitle, Valid: in.SeoTitle != ""},
		SeoDesc:  pgtype.Text{String: in.SeoDesc, Valid: in.SeoDesc != ""},
		Column7:  boolPtrToBool(in.IsPublished, true),
	})
	if err != nil {
		return nil, err
	}
	return &CmsPage{
		Slug:        r.Slug,
		Title:       r.Title,
		BodyHTML:    r.BodyHtml,
		SeoTitle:    r.SeoTitle.String,
		SeoDesc:     r.SeoDesc.String,
		IsPublished: r.IsPublished,
	}, nil
}

func (s *Service) AdminListCmsPages(ctx context.Context) ([]CmsPage, error) {
	rows, err := s.q.AdminListCmsPages(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CmsPage, 0, len(rows))
	for _, r := range rows {
		out = append(out, CmsPage{
			Slug:        r.Slug,
			Title:       r.Title,
			IsPublished: r.IsPublished,
		})
	}
	return out, nil
}

// helpers ------------------------------------------------------------------

func emptyJSONIfBlank(s string) string {
	if strings.TrimSpace(s) == "" {
		return `{"type":"doc","content":[]}`
	}
	return s
}

func slugify(s string) string {
	out := make([]rune, 0, len(s))
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_':
			if !prevDash && len(out) > 0 {
				out = append(out, '-')
				prevDash = true
			}
		}
	}
	return strings.Trim(string(out), "-")
}

// estimateMinutes does a 200-word-per-minute read estimate. Cheap heuristic
// — we're rendering the result as "5 min read" not legal advice.
func estimateMinutes(html string) int32 {
	words := len(strings.Fields(stripTags(html)))
	mins := int32(words / 200)
	if mins < 1 {
		mins = 1
	}
	return mins
}

func stripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func boolPtrToBool(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

