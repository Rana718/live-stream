package auth

import (
	"live-platform/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// ForgotPassword godoc
// @Summary Start a password reset — returns opaque reset token (email integration pending)
// @Tags auth
// @Router /auth/forgot-password [post]
func (h *Handler) ForgotPassword(c fiber.Ctx) error {
	var req struct {
		Email string `json:"email" validate:"required,email"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	token, err := h.service.StartPasswordReset(c.Context(), req.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	// In prod, email `token` instead of returning it. We return it so the flow is testable.
	return c.JSON(fiber.Map{"message": "reset token issued", "token": token})
}

// ResetPassword godoc
// @Summary Complete password reset using the token
// @Tags auth
// @Router /auth/reset-password [post]
func (h *Handler) ResetPassword(c fiber.Ctx) error {
	var req CompletePasswordResetRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.service.CompletePasswordReset(c.Context(), req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "password reset successful"})
}

// SendEmailVerification godoc
// @Summary Request a new email verification token (auth required)
// @Tags auth
// @Security BearerAuth
// @Router /auth/verify-email/start [post]
func (h *Handler) SendEmailVerification(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	token, err := h.service.StartEmailVerification(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "verification token issued", "token": token})
}

// ConfirmEmailVerification godoc
// @Summary Confirm email using the verification token
// @Tags auth
// @Router /auth/verify-email [post]
func (h *Handler) ConfirmEmailVerification(c fiber.Ctx) error {
	var req struct {
		Token string `json:"token" validate:"required,min=16"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.service.CompleteEmailVerification(c.Context(), req.Token); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "email verified"})
}
