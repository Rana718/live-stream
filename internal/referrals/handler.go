package referrals

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// MyCode — GET /api/v1/referrals/me
//
//	@Summary  Current user's referral code + stats
//	@Tags     referrals
//	@Security BearerAuth
//	@Success  200 {object} MyCodeResult
//	@Router   /referrals/me [get]
func (h *Handler) MyCode(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	res, err := h.svc.MyCode(c.Context(), tenantID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}
