package audit

import (
	"strconv"

	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit, offset := int32(50), int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// List — GET /admin/audit  (tenant admin: their tenant's audit log)
func (h *Handler) List(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.List(c.Context(), middleware.CurrentTenantID(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":            utils.UUIDFromPg(r.ID),
			"actor_id":      utils.UUIDFromPg(r.ActorUserID),
			"actor_role":    utils.TextFromPg(r.ActorRole),
			"action":        r.Action,
			"resource_type": utils.TextFromPg(r.EntityType),
			"entity_type":   utils.TextFromPg(r.EntityType),
			"resource_id":   utils.UUIDFromPg(r.EntityID),
			"entity_id":     utils.UUIDFromPg(r.EntityID),
			"created_at":    r.CreatedAt.Time,
		}
	}
	return c.JSON(out)
}
