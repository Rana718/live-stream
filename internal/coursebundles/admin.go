package coursebundles

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type AdminBundleInput struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	PricePaise   int64    `json:"price_paise"`
	PriceMinor   int64    `json:"price_minor"`
	CoverURL     string   `json:"cover_url"`
	DisplayOrder int32    `json:"display_order"`
	CourseIDs    []string `json:"course_ids"`
	IsActive     *bool    `json:"is_active"`
}

func (in AdminBundleInput) price() int64 {
	if in.PriceMinor > 0 {
		return in.PriceMinor
	}
	return in.PricePaise
}

func (s *Service) AdminList(ctx context.Context, tenantID uuid.UUID) ([]BundleView, error) {
	rows, err := s.q.AdminListCourseBundles(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return nil, err
	}
	out := make([]BundleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.bundleView(ctx, tenantID, r.ID, r.Title, utils.TextFromPg(r.Description), r.CoverUrl, r.IsActive))
	}
	return out, nil
}

func (s *Service) ensureCourseProduct(ctx context.Context, q *db.Queries, tenantID, courseID uuid.UUID) (pgtype.UUID, error) {
	if p, err := q.GetProductForCourse(ctx, utils.UUIDToPg(courseID)); err == nil {
		return p.ID, nil
	}
	p, err := q.CreateProduct(ctx, db.CreateProductParams{
		TenantID: utils.UUIDToPg(tenantID), Kind: db.ProductKind("course"), CourseID: utils.UUIDToPg(courseID),
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return p.ID, nil
}

func (s *Service) AdminCreate(ctx context.Context, tenantID uuid.UUID, in AdminBundleInput) (*BundleView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	b, err := q.CreateCourseBundle(ctx, db.CreateCourseBundleParams{
		TenantID: utils.UUIDToPg(tenantID), Title: in.Title,
		Description: ntext(in.Description), CoverUrl: ntext(in.CoverURL),
		DisplayOrder: pgtype.Int4{Int32: in.DisplayOrder, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	prod, err := q.CreateProduct(ctx, db.CreateProductParams{
		TenantID: utils.UUIDToPg(tenantID), Kind: db.ProductKind("bundle"), BundleID: b.ID,
	})
	if err != nil {
		return nil, err
	}
	if in.price() > 0 {
		if _, err := q.UpsertActivePrice(ctx, db.UpsertActivePriceParams{
			TenantID: utils.UUIDToPg(tenantID), ProductID: prod.ID, AmountMinor: in.price(),
		}); err != nil {
			return nil, err
		}
	}
	for i, cidStr := range in.CourseIDs {
		cid, e := uuid.Parse(cidStr)
		if e != nil {
			continue
		}
		cp, e := s.ensureCourseProduct(ctx, q, tenantID, cid)
		if e != nil {
			return nil, e
		}
		if err := q.AddBundleItem(ctx, db.AddBundleItemParams{
			TenantID: utils.UUIDToPg(tenantID), BundleProductID: prod.ID, ItemProductID: cp, Position: int32(i),
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	v := s.bundleView(ctx, tenantID, b.ID, b.Title, utils.TextFromPg(b.Description), b.CoverUrl, b.IsActive)
	return &v, nil
}

func (s *Service) AdminSetActive(ctx context.Context, tenantID, id uuid.UUID, active bool) error {
	return s.q.SetCourseBundleActive(ctx, db.SetCourseBundleActiveParams{
		ID: utils.UUIDToPg(id), TenantID: utils.UUIDToPg(tenantID), IsActive: active,
	})
}

func (s *Service) AdminDelete(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.q.DeleteCourseBundle(ctx, db.DeleteCourseBundleParams{
		ID: utils.UUIDToPg(id), TenantID: utils.UUIDToPg(tenantID),
	})
}

// ── admin handlers ──────────────────────────────────────────────────

func (h *Handler) AdminList(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	rows, err := h.svc.AdminList(c.Context(), tenantID)
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
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		IsActive bool `json:"is_active"`
	}
	_ = c.Bind().Body(&body)
	if err := h.svc.AdminSetActive(c.Context(), tenantID, id, body.IsActive); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"updated": true})
}

func (h *Handler) AdminDelete(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.AdminDelete(c.Context(), tenantID, id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"deleted": true})
}
