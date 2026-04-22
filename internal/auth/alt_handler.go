package auth

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// SendOtp dispatches a new OTP to a phone number. Returns the OTP inline when
// the server runs in dev-mode (see alt_login.go) so QA can skip the SMS leg.
func (h *Handler) SendOtp(c fiber.Ctx) error {
	var req struct {
		Phone string `json:"phone"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	phone, devCode, err := h.service.SendOTP(c.Context(), req.Phone)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	resp := fiber.Map{"phone": phone, "sent": true}
	if devCode != "" {
		resp["dev_code"] = devCode
	}
	return c.JSON(resp)
}

// VerifyOtp consumes a code and returns a full token bundle (access +
// refresh + user). First-time phones auto-create an account that will be
// funneled through the onboarding flow on the very next navigation.
func (h *Handler) VerifyOtp(c fiber.Ctx) error {
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	tokens, err := h.service.LoginWithOTP(c.Context(), req.Phone, req.Code)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(tokens)
}

// GoogleSignIn takes the identity pulled from Google's SDK on the client and
// mints our tokens. Current build trusts the client — phase 2b will verify
// the ID token server-side via the Firebase Admin SDK before trusting sub.
func (h *Handler) GoogleSignIn(c fiber.Ctx) error {
	var req GoogleIdentity
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	tokens, err := h.service.LoginWithGoogle(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(tokens)
}

// LinkPhone lets an already-authenticated account claim a phone number after
// OTP verification. Useful for users who signed up via email/Google and
// later want mobile OTP as a secondary login method.
func (h *Handler) LinkPhone(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if _, err := h.service.LinkPhone(c.Context(), userID, req.Phone, req.Code); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"linked": true})
}

// LinkGoogle attaches a Google identity to the current account. Mirrors
// LinkPhone semantics for users who started with email/OTP.
func (h *Handler) LinkGoogle(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	var req GoogleIdentity
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if _, err := h.service.LinkGoogle(c.Context(), userID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"linked": true})
}
