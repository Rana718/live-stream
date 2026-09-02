package search

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

// Search godoc
// @Summary Full-text search across courses and lectures
// @Tags search
// @Param q query string true "Query string"
// @Param limit query int false "Page size"
// @Param offset query int false "Offset"
// @Router /search [get]
func (h *Handler) Search(c fiber.Ctx) error {
	q := c.Query("q")
	if q == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "q is required"})
	}
	limit := int32(20)
	offset := int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	out, err := h.service.Unified(c.Context(), tenantID, q, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}
