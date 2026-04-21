package subscriptions

import (
	"encoding/json"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

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

func planToMap(p *db.SubscriptionPlan) fiber.Map {
	var features []string
	_ = json.Unmarshal(p.Features, &features)
	return fiber.Map{
		"id":            utils.UUIDFromPg(p.ID),
		"name":          p.Name,
		"slug":          p.Slug,
		"description":   utils.TextFromPg(p.Description),
		"price":         utils.NumericToFloat(p.Price),
		"currency":      utils.TextFromPg(p.Currency),
		"duration_days": p.DurationDays,
		"features":      features,
		"display_order": utils.Int4FromPg(p.DisplayOrder),
	}
}

func subToMap(s *db.UserSubscription) fiber.Map {
	return fiber.Map{
		"id":         utils.UUIDFromPg(s.ID),
		"user_id":    utils.UUIDFromPg(s.UserID),
		"plan_id":    utils.UUIDFromPg(s.PlanID),
		"status":     utils.TextFromPg(s.Status),
		"starts_at":  s.StartsAt,
		"ends_at":    s.EndsAt,
		"auto_renew": utils.BoolFromPg(s.AutoRenew),
	}
}

// ListPlans godoc
// @Summary List all active subscription plans
// @Tags subscriptions
// @Router /subscriptions/plans [get]
func (h *Handler) ListPlans(c fiber.Ctx) error {
	rows, err := h.service.ListActivePlans(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = planToMap(&rows[i])
	}
	return c.JSON(out)
}

// CreatePlan godoc
// @Summary Create a new subscription plan (admin)
// @Tags subscriptions
// @Security BearerAuth
// @Router /subscriptions/plans [post]
func (h *Handler) CreatePlan(c fiber.Ctx) error {
	var req UpsertPlanRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	p, err := h.service.CreatePlan(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(planToMap(p))
}

// Checkout godoc
// @Summary Start checkout for a plan. Returns Razorpay order info.
// @Tags subscriptions
// @Security BearerAuth
// @Router /subscriptions/checkout [post]
func (h *Handler) Checkout(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req CheckoutRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	resp, err := h.service.StartCheckout(c.Context(), userID, req, h.publicKey)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

// Verify godoc
// @Summary Verify Razorpay payment signature and activate subscription
// @Tags subscriptions
// @Security BearerAuth
// @Router /subscriptions/verify [post]
func (h *Handler) Verify(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req VerifyRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	sub, err := h.service.VerifyCheckout(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(subToMap(sub))
}

// GetMine godoc
// @Summary Get my active subscription
// @Tags subscriptions
// @Security BearerAuth
// @Router /subscriptions/me [get]
func (h *Handler) GetMine(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	sub, err := h.service.GetActive(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no active subscription"})
	}
	return c.JSON(subToMap(sub))
}

// ListMyHistory godoc
// @Summary List all my past + current subscriptions
// @Tags subscriptions
// @Security BearerAuth
// @Router /subscriptions/history [get]
func (h *Handler) ListMyHistory(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	rows, err := h.service.ListMine(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":         utils.UUIDFromPg(r.ID),
			"plan_id":    utils.UUIDFromPg(r.PlanID),
			"plan_name":  r.PlanName,
			"plan_slug":  r.PlanSlug,
			"status":     utils.TextFromPg(r.Status),
			"starts_at":  r.StartsAt,
			"ends_at":    r.EndsAt,
			"auto_renew": utils.BoolFromPg(r.AutoRenew),
		}
	}
	return c.JSON(out)
}

// Cancel godoc
// @Summary Cancel a subscription
// @Tags subscriptions
// @Security BearerAuth
// @Router /subscriptions/{id}/cancel [post]
func (h *Handler) Cancel(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Cancel(c.Context(), userID, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "cancelled"})
}

// Webhook godoc
// @Summary Razorpay webhook endpoint (server-to-server)
// @Tags subscriptions
// @Router /subscriptions/webhook [post]
func (h *Handler) Webhook(c fiber.Ctx) error {
	sig := c.Get("X-Razorpay-Signature")
	body := c.Body()
	if err := h.service.HandleWebhook(c.Context(), body, sig); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}
