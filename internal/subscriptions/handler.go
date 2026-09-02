package subscriptions

import (
	"live-platform/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service   *Service
	publicKey string
}

func NewHandler(s *Service, razorpayPublicKey string) *Handler {
	return &Handler{service: s, publicKey: razorpayPublicKey}
}

func (h *Handler) tenant(c fiber.Ctx) uuid.UUID { return middleware.CurrentTenantID(c) }
func (h *Handler) user(c fiber.Ctx) uuid.UUID   { return middleware.CurrentUserID(c) }

// ListPlans — GET /subscriptions/plans
func (h *Handler) ListPlans(c fiber.Ctx) error {
	rows, err := h.service.ListActivePlans(c.Context(), h.tenant(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// CreatePlan — POST /subscriptions/plans  (admin)
func (h *Handler) CreatePlan(c fiber.Ctx) error {
	var req UpsertPlanRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	p, err := h.service.CreatePlan(c.Context(), h.tenant(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(p)
}

// Checkout — POST /subscriptions/checkout
func (h *Handler) Checkout(c fiber.Ctx) error {
	var req CheckoutRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	resp, err := h.service.StartCheckout(c.Context(), h.tenant(c), h.user(c), req, h.publicKey)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

// Verify — POST /subscriptions/verify
func (h *Handler) Verify(c fiber.Ctx) error {
	var req VerifyRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	sub, err := h.service.VerifyCheckout(c.Context(), h.user(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(sub)
}

// GetMine — GET /subscriptions/me
func (h *Handler) GetMine(c fiber.Ctx) error {
	sub, err := h.service.GetActive(c.Context(), h.tenant(c), h.user(c))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no active subscription"})
	}
	return c.JSON(sub)
}

// ListMyHistory — GET /subscriptions/history
func (h *Handler) ListMyHistory(c fiber.Ctx) error {
	rows, err := h.service.ListMine(c.Context(), h.tenant(c), h.user(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// Cancel — POST /subscriptions/:id/cancel
func (h *Handler) Cancel(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Cancel(c.Context(), h.tenant(c), h.user(c), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "cancelled"})
}

// Webhook — POST /subscriptions/webhook (superseded by internal/webhooks)
func (h *Handler) Webhook(c fiber.Ctx) error {
	_ = h.service.HandleWebhook(c.Context(), c.Body(), c.Get("X-Razorpay-Signature"))
	return c.JSON(fiber.Map{"status": "ok"})
}
