package banners

import (
	"strconv"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func toMap(b *db.Banner) fiber.Map {
	return fiber.Map{
		"id":               utils.UUIDFromPg(b.ID),
		"title":            b.Title,
		"subtitle":         utils.TextFromPg(b.Subtitle),
		"image_url":        b.ImageUrl,
		"background_color": utils.TextFromPg(b.BackgroundColor),
		"link_type":        utils.TextFromPg(b.LinkType),
		"link_id":          utils.UUIDFromPg(b.LinkID),
		"link_url":         utils.TextFromPg(b.LinkUrl),
		"display_order":    utils.Int4FromPg(b.DisplayOrder),
		"is_active":        utils.BoolFromPg(b.IsActive),
		"starts_at":        b.StartsAt,
		"ends_at":          b.EndsAt,
		"created_at":       b.CreatedAt,
	}
}

// ListActive godoc
// @Summary List active home-page banners (public, respects schedule window)
// @Tags banners
// @Router /banners [get]
func (h *Handler) ListActive(c fiber.Ctx) error {
	limit := int32(10)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 30 {
		limit = int32(l)
	}
	rows, err := h.service.ListActive(c.Context(), limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = toMap(&rows[i])
	}
	return c.JSON(out)
}

// ListAll godoc
// @Summary List all banners (admin)
// @Tags banners
// @Security BearerAuth
// @Router /admin/banners [get]
func (h *Handler) ListAll(c fiber.Ctx) error {
	limit := int32(50)
	offset := int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	rows, err := h.service.ListAll(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = toMap(&rows[i])
	}
	return c.JSON(out)
}

// Create godoc
// @Summary Create a banner (admin)
// @Tags banners
// @Security BearerAuth
// @Router /admin/banners [post]
func (h *Handler) Create(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req UpsertBannerRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	b, err := h.service.Create(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(toMap(b))
}

// Update godoc
// @Summary Update a banner (admin)
// @Tags banners
// @Security BearerAuth
// @Router /admin/banners/{id} [put]
func (h *Handler) Update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req UpsertBannerRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	b, err := h.service.Update(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(toMap(b))
}

// SetActive godoc
// @Summary Activate / deactivate a banner (admin)
// @Tags banners
// @Security BearerAuth
// @Router /admin/banners/{id}/active [post]
func (h *Handler) SetActive(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Active bool `json:"active"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	b, err := h.service.SetActive(c.Context(), id, req.Active)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(toMap(b))
}

// Delete godoc
// @Summary Delete a banner (admin)
// @Tags banners
// @Security BearerAuth
// @Router /admin/banners/{id} [delete]
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
