package coupons

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Apply handles POST /api/v1/coupons/apply — students hit it from checkout
// to preview the discount before they go to Razorpay.
//
//	@Summary	Validate a coupon code against an in-progress checkout
//	@Tags		coupons
//	@Security	BearerAuth
//	@Param		body	body	applyRequest	true	"Code + amount + scope context"
//	@Success	200		{object}	ApplyResult
//	@Router		/coupons/apply [post]
func (h *Handler) Apply(c fiber.Ctx) error {
	var req applyRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	userID, _ := c.Locals("userID").(uuid.UUID)
	var courseID *uuid.UUID
	if req.CourseID != "" {
		if u, err := uuid.Parse(req.CourseID); err == nil {
			courseID = &u
		}
	}
	res, err := h.svc.Apply(c.Context(), tenantID, userID, req.Code, req.AmountPaise, courseID, req.Subscription)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

type applyRequest struct {
	Code         string `json:"code"`
	AmountPaise  int    `json:"amount_paise"`
	CourseID     string `json:"course_id"`
	Subscription bool   `json:"subscription"`
}

// AdminCreate handles POST /api/v1/admin/coupons.
func (h *Handler) AdminCreate(c fiber.Ctx) error {
	var in CreateInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	row, err := h.svc.Create(c.Context(), tenantID, in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(row)
}

func (h *Handler) AdminList(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	rows, err := h.svc.List(c.Context(), tenantID, 100, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

func (h *Handler) AdminSetActive(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		IsActive bool `json:"is_active"`
	}
	_ = c.Bind().Body(&body)
	if err := h.svc.SetActive(c.Context(), id, body.IsActive); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"updated": true})
}

func (h *Handler) AdminDelete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.Delete(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"deleted": true})
}
