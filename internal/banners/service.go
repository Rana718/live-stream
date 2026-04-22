package banners

import (
	"context"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type UpsertBannerRequest struct {
	Title           string     `json:"title" validate:"required,min=2"`
	Subtitle        string     `json:"subtitle"`
	ImageURL        string     `json:"image_url" validate:"required,url"`
	BackgroundColor string     `json:"background_color"`
	LinkType        string     `json:"link_type"`           // course | lecture | test | url | none
	LinkID          *uuid.UUID `json:"link_id"`
	LinkURL         string     `json:"link_url"`
	DisplayOrder    int32      `json:"display_order"`
	IsActive        bool       `json:"is_active"`
	StartsAt        *time.Time `json:"starts_at"`
	EndsAt          *time.Time `json:"ends_at"`
}

func (s *Service) Create(ctx context.Context, creator uuid.UUID, req UpsertBannerRequest) (*db.Banner, error) {
	b, err := s.q.CreateBanner(ctx, db.CreateBannerParams{
		Title:           req.Title,
		Subtitle:        utils.TextToPg(req.Subtitle),
		ImageUrl:        req.ImageURL,
		BackgroundColor: utils.TextToPg(req.BackgroundColor),
		LinkType:        utils.TextToPg(req.LinkType),
		LinkID:          utils.UUIDPtrToPg(req.LinkID),
		LinkUrl:         utils.TextToPg(req.LinkURL),
		DisplayOrder:    utils.Int4ToPg(req.DisplayOrder),
		StartsAt:        utils.TimestampPtrToPg(req.StartsAt),
		EndsAt:          utils.TimestampPtrToPg(req.EndsAt),
		CreatedBy:       utils.UUIDToPg(creator),
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpsertBannerRequest) (*db.Banner, error) {
	b, err := s.q.UpdateBanner(ctx, db.UpdateBannerParams{
		ID:              utils.UUIDToPg(id),
		Title:           req.Title,
		Subtitle:        utils.TextToPg(req.Subtitle),
		ImageUrl:        req.ImageURL,
		BackgroundColor: utils.TextToPg(req.BackgroundColor),
		LinkType:        utils.TextToPg(req.LinkType),
		LinkID:          utils.UUIDPtrToPg(req.LinkID),
		LinkUrl:         utils.TextToPg(req.LinkURL),
		DisplayOrder:    utils.Int4ToPg(req.DisplayOrder),
		IsActive:        utils.BoolToPg(req.IsActive),
		StartsAt:        utils.TimestampPtrToPg(req.StartsAt),
		EndsAt:          utils.TimestampPtrToPg(req.EndsAt),
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Service) SetActive(ctx context.Context, id uuid.UUID, active bool) (*db.Banner, error) {
	b, err := s.q.SetBannerActive(ctx, db.SetBannerActiveParams{
		ID:       utils.UUIDToPg(id),
		IsActive: utils.BoolToPg(active),
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Service) ListActive(ctx context.Context, limit int32) ([]db.Banner, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.q.ListActiveBanners(ctx, limit)
}

func (s *Service) ListAll(ctx context.Context, limit, offset int32) ([]db.Banner, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.q.ListAllBanners(ctx, db.ListAllBannersParams{Limit: limit, Offset: offset})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteBanner(ctx, utils.UUIDToPg(id))
}
