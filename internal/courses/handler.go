package courses

import (
	"strconv"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit, offset := int32(20), int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// fullView renders a GetCourseRow with legacy-compatible keys (description,
// is_published) so the current frontend keeps working during the migration.
func fullView(c db.GetCourseRow) fiber.Map {
	return fiber.Map{
		"id":                 utils.UUIDFromPg(c.ID),
		"tenant_id":          utils.UUIDFromPg(c.TenantID),
		"exam_category_id":   utils.UUIDFromPg(c.ExamCategoryID),
		"title":              c.Title,
		"slug":               c.Slug,
		"summary":            utils.TextFromPg(c.Summary),
		"description":        utils.TextFromPg(c.Summary),
		"description_rich":   json(c.DescriptionRich),
		"thumbnail_url":      utils.TextFromPg(c.ThumbnailUrl),
		"promo_video_url":    utils.TextFromPg(c.PromoVideoUrl),
		"language":           c.Language,
		"level":              c.Level,
		"class_level":        utils.TextFromPg(c.ClassLevel),
		"exam_goal":          utils.TextFromPg(c.ExamGoal),
		"status":             string(c.Status),
		"is_published":       c.Status == db.PublishStatusPublished,
		"approval_status":    c.ApprovalStatus,
		"hsn_sac":            utils.TextFromPg(c.HsnSac),
		"tax_rate_bps":       c.TaxRateBps,
		"refund_window_days": c.RefundWindowDays,
		"created_by":         utils.UUIDFromPg(c.CreatedBy),
		"created_at":         c.CreatedAt.Time,
		"updated_at":         c.UpdatedAt.Time,
	}
}

// priceMinorFor returns the course's active price in paise, 0 if free/unpriced.
func (h *Handler) priceMinorFor(c fiber.Ctx, courseID uuid.UUID) int64 {
	p, err := h.service.GetPrice(c.Context(), courseID)
	if err != nil {
		return 0
	}
	return p
}

func json(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// List — GET /courses. Published catalogue for the caller's tenant.
func (h *Handler) List(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	tenantID := middleware.CurrentTenantID(c)

	if q := c.Query("q"); q != "" {
		rows, err := h.service.Search(c.Context(), tenantID, q, limit, offset)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out := make([]fiber.Map, len(rows))
		for i, r := range rows {
			out[i] = fiber.Map{
				"id": utils.UUIDFromPg(r.ID), "title": r.Title, "slug": r.Slug,
				"summary": utils.TextFromPg(r.Summary), "description": utils.TextFromPg(r.Summary),
				"thumbnail_url": utils.TextFromPg(r.ThumbnailUrl),
			}
		}
		return c.JSON(out)
	}

	rows, err := h.service.ListPublished(c.Context(), tenantID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "title": r.Title, "slug": r.Slug,
			"summary": utils.TextFromPg(r.Summary), "description": utils.TextFromPg(r.Summary),
			"thumbnail_url": utils.TextFromPg(r.ThumbnailUrl),
			"language":      r.Language, "level": r.Level,
			"class_level": utils.TextFromPg(r.ClassLevel), "exam_goal": utils.TextFromPg(r.ExamGoal),
			"is_published": true,
			"price_minor":  r.PriceMinor, "price": float64(r.PriceMinor) / 100,
		}
	}
	return c.JSON(out)
}

// ListForAdmin — GET /admin/courses  (every course incl. drafts + pending)
func (h *Handler) ListForAdmin(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListForAdmin(c.Context(), middleware.CurrentTenantID(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "title": r.Title, "slug": r.Slug,
			"summary": utils.TextFromPg(r.Summary), "description": utils.TextFromPg(r.Summary),
			"thumbnail_url": utils.TextFromPg(r.ThumbnailUrl),
			"language":      r.Language, "level": r.Level,
			"status": string(r.Status), "approval_status": string(r.ApprovalStatus),
			"is_published":   r.Status == db.PublishStatusPublished,
			"enrolled_count": r.EnrolledCount,
			"price_minor":    r.PriceMinor, "price": float64(r.PriceMinor) / 100,
			"created_at": r.CreatedAt.Time,
		}
	}
	return c.JSON(out)
}

// Get — GET /courses/:id  (id or slug)
func (h *Handler) Get(c fiber.Ctx) error {
	idParam := c.Params("id")
	if id, err := uuid.Parse(idParam); err == nil {
		course, err := h.service.Get(c.Context(), id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
		}
		return c.JSON(withPrice(fullView(course), h.priceMinorFor(c, id)))
	}
	row, err := h.service.GetBySlug(c.Context(), middleware.CurrentTenantID(c), idParam)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	// GetCourseBySlugRow is a subset; re-fetch full by id for a consistent shape.
	cid := uuid.UUID(row.ID.Bytes)
	course, err := h.service.Get(c.Context(), cid)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(withPrice(fullView(course), h.priceMinorFor(c, cid)))
}

func withPrice(m fiber.Map, minor int64) fiber.Map {
	m["price_minor"] = minor
	m["price"] = float64(minor) / 100
	return m
}

// Create — POST /courses  (instructor/admin)
func (h *Handler) Create(c fiber.Ctx) error {
	var req CreateCourseRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	row, err := h.service.Create(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(row)
}

// Update — PUT /courses/:id
func (h *Handler) Update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req CreateCourseRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	row, err := h.service.Update(c.Context(), middleware.CurrentTenantID(c), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if req.Description != "" || req.Summary != "" {
		// no-op: summary handled in Update
	}
	// honour an is_published toggle if the client sent one
	if v := c.Query("publish"); v == "true" || v == "false" {
		_ = h.service.SetPublished(c.Context(), id, v == "true")
	}
	return c.JSON(row)
}

// SetPublished — PATCH /courses/:id/publish  { is_published: bool }
func (h *Handler) SetPublished(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		IsPublished bool `json:"is_published"`
	}
	_ = c.Bind().JSON(&body)
	if err := h.service.SetPublished(c.Context(), id, body.IsPublished); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"id": id, "is_published": body.IsPublished})
}

// Delete — DELETE /courses/:id  (admin, soft)
func (h *Handler) Delete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Delete(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}
