package courseorders

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	svc   *Service
	keyID string
}

func NewHandler(svc *Service, keyID string) *Handler {
	return &Handler{svc: svc, keyID: keyID}
}

// Buy handles POST /api/v1/courses/:id/buy.
//
//	@Summary	Start a Razorpay checkout for a course
//	@Tags		courseorders
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Course UUID"
//	@Success	200	{object}	CreateOrderResult
//	@Router		/courses/{id}/buy [post]
func (h *Handler) Buy(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	userID, _ := c.Locals("userID").(uuid.UUID)
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)

	res, err := h.svc.Buy(c.Context(), tenantID, userID, courseID, h.keyID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

// Verify handles POST /api/v1/payments/verify.
//
//	@Summary	Verify a Razorpay checkout signature & enroll
//	@Tags		courseorders
//	@Security	BearerAuth
//	@Param		body	body	VerifyRequest	true	"Razorpay handler payload"
//	@Success	200		{object}	map[string]interface{}
//	@Router		/payments/verify [post]
func (h *Handler) Verify(c fiber.Ctx) error {
	var req VerifyRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	userID, _ := c.Locals("userID").(uuid.UUID)
	row, err := h.svc.Verify(c.Context(), req, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": row.Status.String, "payment": row})
}
