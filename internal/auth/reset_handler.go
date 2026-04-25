package auth

import "github.com/gofiber/fiber/v3"

// All four endpoints below were part of the email/password flow that has
// been retired. They stay registered (so old clients hit a deterministic
// 410 with a hint) but no longer touch the database.
//
// New flows: phone OTP for everyday auth, Google sign-in for one-tap.
// Both are in alt_handler.go / alt_login.go.

// ForgotPassword: deprecated.
// @Summary  [deprecated] Email password reset
// @Tags     auth
// @Failure  410 {object} map[string]interface{}
// @Router   /auth/forgot-password [post]
func (h *Handler) ForgotPassword(c fiber.Ctx) error {
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error": "password-based auth removed",
		"hint":  "use POST /auth/otp/send for phone OTP",
	})
}

// ResetPassword: deprecated.
// @Summary  [deprecated] Email password reset confirmation
// @Tags     auth
// @Router   /auth/reset-password [post]
func (h *Handler) ResetPassword(c fiber.Ctx) error {
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error": "password-based auth removed",
	})
}

// SendEmailVerification: deprecated. We don't ask for email at signup
// anymore. Email is optional metadata the user can attach later via the
// linking flow.
// @Summary  [deprecated] Email verification start
// @Router   /auth/verify-email/start [post]
func (h *Handler) SendEmailVerification(c fiber.Ctx) error {
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error": "email verification removed — phone is the primary identity",
	})
}

// ConfirmEmailVerification: deprecated.
// @Summary  [deprecated] Email verification completion
// @Router   /auth/verify-email [post]
func (h *Handler) ConfirmEmailVerification(c fiber.Ctx) error {
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error": "email verification removed — phone is the primary identity",
	})
}
