package coursebundles

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	svc   *Service
	keyID string
}

func NewHandler(svc *Service, razorpayKeyID string) *Handler {
	return &Handler{svc: svc, keyID: razorpayKeyID}
}

// List — GET /api/v1/bundles
//
//	@Summary	Active course bundles for the current tenant
//	@Tags		bundles
//	@Security	BearerAuth
func (h *Handler) List(c fiber.Ctx) error {
	rows, err := h.svc.List(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// Buy — POST /api/v1/bundles/:id/buy
//
//	@Summary	Create a Razorpay order for a bundle
//	@Tags		bundles
//	@Security	BearerAuth
func (h *Handler) Buy(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	userID, _ := c.Locals("userID").(uuid.UUID)
	bundleID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	res, err := h.svc.Buy(c.Context(), tenantID, userID, bundleID, h.keyID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// Verify — POST /api/v1/bundles/verify
//
//	@Summary	Confirm bundle payment + enroll in every course
//	@Tags		bundles
//	@Security	BearerAuth
func (h *Handler) Verify(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req VerifyRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if err := h.svc.Verify(c.Context(), req, userID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"verified": true})
}
