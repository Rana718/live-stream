package refunds

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Issue — POST /api/v1/admin/refunds
//
// Tenant-admin endpoint. Validates ownership, hits Razorpay's refund
// API with the internal payment UUID as the idempotency key, patches
// the row to status=refunded, sends the user an email receipt.
//
//	@Summary  Issue a refund on a paid order
//	@Tags     refunds
//	@Security BearerAuth
//	@Param    body body IssueInput true "Payment id + amount + reason"
//	@Success  200  {object} IssueResult
//	@Router   /admin/refunds [post]
func (h *Handler) Issue(c fiber.Ctx) error {
	var in IssueInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	res, err := h.svc.Issue(c.Context(), tenantID, in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}
