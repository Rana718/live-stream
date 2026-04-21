package audit

import (
	"net/netip"
	"strconv"

	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
)

func ipString(a *netip.Addr) string {
	if a == nil {
		return ""
	}
	return a.String()
}

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit := int32(50)
	offset := int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// List godoc
// @Summary List recent audit log entries (admin)
// @Tags admin
// @Security BearerAuth
// @Router /admin/audit [get]
func (h *Handler) List(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.List(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":            utils.UUIDFromPg(r.ID),
			"actor_id":      utils.UUIDFromPg(r.ActorID),
			"actor_role":    utils.TextFromPg(r.ActorRole),
			"action":        r.Action,
			"resource_type": utils.TextFromPg(r.ResourceType),
			"resource_id":   utils.UUIDFromPg(r.ResourceID),
			"ip":            ipString(r.Ip),
			"user_agent":    utils.TextFromPg(r.UserAgent),
			"created_at":    r.CreatedAt,
		}
	}
	return c.JSON(out)
}
