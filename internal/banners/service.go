// Package banners — schema-v2. tenant-scoped; timestamptz schedule window.
package banners

import (
	"context"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

func tsz(t *time.Time) pgtype.Timestamptz {
	if t == nil || t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
func ntext(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

type UpsertBannerRequest struct {
	Title           string     `json:"title" validate:"required,min=2"`
	Subtitle        string     `json:"subtitle"`
	ImageURL        string     `json:"image_url" validate:"required,url"`
	BackgroundColor string     `json:"background_color"`
	LinkType        string     `json:"link_type"`
	LinkID          *uuid.UUID `json:"link_id"`
	LinkURL         string     `json:"link_url"`
	DisplayOrder    int32      `json:"display_order"`
	IsActive        bool       `json:"is_active"`
	StartsAt        *time.Time `json:"starts_at"`
	EndsAt          *time.Time `json:"ends_at"`
}

// Banner is the flattened client view.
type Banner struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Subtitle        string     `json:"subtitle"`
	ImageURL        string     `json:"image_url"`
	BackgroundColor string     `json:"background_color"`
	LinkType        string     `json:"link_type"`
	LinkID          string     `json:"link_id"`
	LinkURL         string     `json:"link_url"`
	DisplayOrder    int32      `json:"display_order"`
	IsActive        bool       `json:"is_active"`
	StartsAt        *time.Time `json:"starts_at"`
	EndsAt          *time.Time `json:"ends_at"`
}

func (s *Service) Create(ctx context.Context, tenantID, creator uuid.UUID, req UpsertBannerRequest) (Banner, error) {
	row, err := s.q.CreateBanner(ctx, db.CreateBannerParams{
		TenantID:        utils.UUIDToPg(tenantID),
		Title:           req.Title,
		ImageUrl:        req.ImageURL,
		Subtitle:        ntext(req.Subtitle),
		BackgroundColor: ntext(req.BackgroundColor),
		LinkType:        ntext(req.LinkType),
		LinkID:          utils.UUIDPtrToPg(req.LinkID),
		LinkUrl:         ntext(req.LinkURL),
		DisplayOrder:    utils.Int4ToPg(req.DisplayOrder),
		StartsAt:        tsz(req.StartsAt),
		EndsAt:          tsz(req.EndsAt),
		CreatedBy:       utils.UUIDToPg(creator),
	})
	if err != nil {
		return Banner{}, err
	}
	return Banner{ID: utils.UUIDFromPg(row.ID), Title: row.Title, ImageURL: row.ImageUrl,
		DisplayOrder: row.DisplayOrder, IsActive: row.IsActive}, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpsertBannerRequest) (Banner, error) {
	row, err := s.q.UpdateBanner(ctx, db.UpdateBannerParams{
		ID:              utils.UUIDToPg(id),
		Title:           ntext(req.Title),
		Subtitle:        ntext(req.Subtitle),
		ImageUrl:        ntext(req.ImageURL),
		BackgroundColor: ntext(req.BackgroundColor),
		LinkType:        ntext(req.LinkType),
		LinkID:          utils.UUIDPtrToPg(req.LinkID),
		LinkUrl:         ntext(req.LinkURL),
		DisplayOrder:    utils.Int4ToPg(req.DisplayOrder),
		IsActive:        pgtype.Bool{Bool: req.IsActive, Valid: true},
		StartsAt:        tsz(req.StartsAt),
		EndsAt:          tsz(req.EndsAt),
	})
	if err != nil {
		return Banner{}, err
	}
	return flatten(row.ID, row.Title, row.Subtitle, row.ImageUrl, row.BackgroundColor, row.LinkType,
		row.LinkID, row.LinkUrl, row.DisplayOrder, row.IsActive, row.StartsAt, row.EndsAt), nil
}

func (s *Service) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	return s.q.SetBannerActive(ctx, db.SetBannerActiveParams{ID: utils.UUIDToPg(id), IsActive: active})
}

func (s *Service) ListActive(ctx context.Context, tenantID uuid.UUID) ([]Banner, error) {
	rows, err := s.q.ListBanners(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return nil, err
	}
	out := make([]Banner, 0, len(rows))
	for _, r := range rows {
		out = append(out, Banner{
			ID: utils.UUIDFromPg(r.ID), Title: r.Title, Subtitle: utils.TextFromPg(r.Subtitle),
			ImageURL: r.ImageUrl, BackgroundColor: utils.TextFromPg(r.BackgroundColor),
			LinkType: utils.TextFromPg(r.LinkType), LinkID: utils.UUIDFromPg(r.LinkID),
			LinkURL: utils.TextFromPg(r.LinkUrl), DisplayOrder: r.DisplayOrder, IsActive: true,
		})
	}
	return out, nil
}

func (s *Service) ListAll(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]Banner, error) {
	rows, err := s.q.ListAllBanners(ctx, db.ListAllBannersParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Banner, 0, len(rows))
	for _, r := range rows {
		out = append(out, flatten(r.ID, r.Title, r.Subtitle, r.ImageUrl, r.BackgroundColor, r.LinkType,
			r.LinkID, r.LinkUrl, r.DisplayOrder, r.IsActive, r.StartsAt, r.EndsAt))
	}
	return out, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteBanner(ctx, utils.UUIDToPg(id))
}

func flatten(id pgtype.UUID, title string, sub pgtype.Text, img string, bg, lt pgtype.Text,
	lid pgtype.UUID, lurl pgtype.Text, order int32, active bool, starts, ends pgtype.Timestamptz) Banner {
	b := Banner{
		ID: utils.UUIDFromPg(id), Title: title, Subtitle: utils.TextFromPg(sub), ImageURL: img,
		BackgroundColor: utils.TextFromPg(bg), LinkType: utils.TextFromPg(lt),
		LinkID: utils.UUIDFromPg(lid), LinkURL: utils.TextFromPg(lurl),
		DisplayOrder: order, IsActive: active,
	}
	if starts.Valid {
		t := starts.Time
		b.StartsAt = &t
	}
	if ends.Valid {
		t := ends.Time
		b.EndsAt = &t
	}
	return b
}
