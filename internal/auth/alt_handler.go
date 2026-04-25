package auth

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// SendOtp dispatches a new OTP for a phone+org_code pair. Returns the OTP
// inline when devModeOTP is on so QA can skip the SMS leg.
func (h *Handler) SendOtp(c fiber.Ctx) error {
	var req struct {
		Phone   string `json:"phone"`
		OrgCode string `json:"org_code"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	phone, devCode, err := h.service.SendOTP(c.Context(), req.Phone, req.OrgCode)
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
// refresh + user) scoped to the supplied tenant. First-time phones auto-
// create an account, with onboarding triggered on the next request.
func (h *Handler) VerifyOtp(c fiber.Ctx) error {
	var req struct {
		Phone        string `json:"phone"`
		Code         string `json:"code"`
		OrgCode      string `json:"org_code"`
		ReferralCode string `json:"referral_code"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	tokens, err := h.service.LoginWithOTP(c.Context(), req.Phone, req.Code, req.OrgCode, req.ReferralCode)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(tokens)
}

// GoogleSignIn takes the identity pulled from Google's SDK on the client and
// mints our tokens, scoped to the tenant resolved from the org_code field.
// Current build trusts the client — phase 2b will verify the ID token via
// the Firebase Admin SDK before trusting sub.
func (h *Handler) GoogleSignIn(c fiber.Ctx) error {
	var req struct {
		GoogleIdentity
		OrgCode string `json:"org_code"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	tokens, err := h.service.LoginWithGoogle(c.Context(), req.GoogleIdentity, req.OrgCode)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(tokens)
}

// LinkPhone lets an already-authenticated account claim a phone number after
// OTP verification. Useful for users who signed up via Google and later want
// mobile OTP as a secondary login method.
func (h *Handler) LinkPhone(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	var req struct {
		Phone   string `json:"phone"`
		Code    string `json:"code"`
		OrgCode string `json:"org_code"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if _, err := h.service.LinkPhone(c.Context(), userID, tenantID, req.Phone, req.Code, req.OrgCode); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"linked": true})
}

// LinkGoogle attaches a Google identity to the current account.
func (h *Handler) LinkGoogle(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	var req GoogleIdentity
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if _, err := h.service.LinkGoogle(c.Context(), userID, tenantID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"linked": true})
}
