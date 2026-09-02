package refunds

import (
	"live-platform/internal/middleware"

	"github.com/gofiber/fiber/v3"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Issue — POST /admin/refunds  (tenant admin)
func (h *Handler) Issue(c fiber.Ctx) error {
	var in IssueInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	res, err := h.svc.Issue(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}
