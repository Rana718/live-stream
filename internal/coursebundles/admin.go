package coursebundles

import (
	"context"
	"fmt"
	"strings"

	"live-platform/internal/database/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Admin-side bundle CRUD. Lives next to the public service so we share
// the existing q (Queries) and BundleView shapes — keeps the JSON shape
// the admin UI sees identical to what students get on the store.

type AdminBundleInput struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	PricePaise   int32    `json:"price_paise"`
	CoverURL     string   `json:"cover_url"`
	DisplayOrder int32    `json:"display_order"`
	CourseIDs    []string `json:"course_ids"`
	IsActive     *bool    `json:"is_active"`
}

func (s *Service) AdminList(ctx context.Context) ([]BundleView, error) {
	rows, err := s.q.AdminListBundles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BundleView, 0, len(rows))
	for _, r := range rows {
		ids := decodeUUIDArray(r.CourseIds)
		// AdminListBundles returns member_price_paise as int32 directly.
		member := r.MemberPricePaise
		save := member - r.PricePaise
		if save < 0 {
			save = 0
		}
		out = append(out, BundleView{
			ID:               uuid.UUID(r.ID.Bytes).String(),
			Title:            r.Title,
			Description:      r.Description.String,
			PricePaise:       r.PricePaise,
			MemberPricePaise: member,
			SavePaise:        save,
			CourseIDs:        ids,
			CoverURL:         r.CoverUrl.String,
		})
	}
	return out, nil
}

func (s *Service) AdminCreate(ctx context.Context, tenantID uuid.UUID, in AdminBundleInput) (*BundleView, error) {
	if strings.TrimSpace(in.Title) == "" || in.PricePaise <= 0 {
		return nil, fmt.Errorf("title and positive price are required")
	}
	if len(in.CourseIDs) < 1 {
		return nil, fmt.Errorf("at least one course is required")
	}
	row, err := s.q.CreateCourseBundle(ctx, db.CreateCourseBundleParams{
		TenantID:     pgtype.UUID{Bytes: tenantID, Valid: true},
		Title:        in.Title,
		Description:  pgtype.Text{String: in.Description, Valid: in.Description != ""},
		PricePaise:   in.PricePaise,
		CoverUrl:     pgtype.Text{String: in.CoverURL, Valid: in.CoverURL != ""},
		DisplayOrder: in.DisplayOrder,
		Column7:      true,
	})
	if err != nil {
		return nil, err
	}
	for _, cid := range in.CourseIDs {
		u, err := uuid.Parse(cid)
		if err != nil {
			continue
		}
		_ = s.q.AddCourseToBundle(ctx, db.AddCourseToBundleParams{
			BundleID: row.ID,
			CourseID: pgtype.UUID{Bytes: u, Valid: true},
		})
	}
	return &BundleView{
		ID:          uuid.UUID(row.ID.Bytes).String(),
		Title:       row.Title,
		Description: row.Description.String,
		PricePaise:  row.PricePaise,
		CoverURL:    row.CoverUrl.String,
		CourseIDs:   in.CourseIDs,
	}, nil
}

func (s *Service) AdminSetActive(ctx context.Context, id uuid.UUID, active bool) error {
	return s.q.SetBundleActive(ctx, db.SetBundleActiveParams{
		ID:       pgtype.UUID{Bytes: id, Valid: true},
		IsActive: active,
	})
}

func (s *Service) AdminDelete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteCourseBundle(ctx, pgtype.UUID{Bytes: id, Valid: true})
}

// Admin handlers --------------------------------------------------------

func (h *Handler) AdminList(c fiber.Ctx) error {
	rows, err := h.svc.AdminList(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

func (h *Handler) AdminCreate(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	var in AdminBundleInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	row, err := h.svc.AdminCreate(c.Context(), tenantID, in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(row)
}

func (h *Handler) AdminSetActive(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if err := h.svc.AdminSetActive(c.Context(), id, body.IsActive); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"updated": true})
}

func (h *Handler) AdminDelete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.AdminDelete(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"deleted": true})
}
