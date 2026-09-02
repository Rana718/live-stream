package auth

import (
	"live-platform/internal/database"
	"live-platform/internal/database/db"
	"live-platform/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func clientMeta(c fiber.Ctx) (ua, ip string) {
	return c.Get("User-Agent"), c.IP()
}

// POST /auth/otp/send
func (h *Handler) SendOTP(c fiber.Ctx) error {
	var req struct {
		Phone   string `json:"phone"`
		OrgCode string `json:"org_code"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	phone, dev, err := h.svc.SendOTP(c.Context(), req.Phone, req.OrgCode)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	resp := fiber.Map{"phone": phone, "sent": true}
	if dev != "" {
		resp["dev_code"] = dev
	}
	return c.JSON(resp)
}

// POST /auth/otp/verify
func (h *Handler) VerifyOTP(c fiber.Ctx) error {
	var req struct {
		Phone        string `json:"phone"`
		Code         string `json:"code"`
		OrgCode      string `json:"org_code"`
		ReferralCode string `json:"referral_code"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	ua, ip := clientMeta(c)
	bundle, err := h.svc.VerifyOTP(c.Context(), req.Phone, req.Code, req.OrgCode, req.ReferralCode, ua, ip)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(bundle)
}

// POST /auth/google
func (h *Handler) GoogleSignIn(c fiber.Ctx) error {
	var req struct {
		GoogleCredential
		OrgCode      string `json:"org_code"`
		ReferralCode string `json:"referral_code"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	ua, ip := clientMeta(c)
	bundle, err := h.svc.GoogleLogin(c.Context(), req.IDToken, req.OrgCode, req.ReferralCode, ua, ip)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(bundle)
}

// POST /auth/refresh
func (h *Handler) Refresh(c fiber.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind().JSON(&req); err != nil || req.RefreshToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "refresh token required"})
	}
	ua, ip := clientMeta(c)
	bundle, err := h.svc.Refresh(c.Context(), req.RefreshToken, ua, ip)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(bundle)
}

// POST /auth/logout
func (h *Handler) Logout(c fiber.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.Bind().JSON(&req)
	if req.RefreshToken != "" {
		_ = h.svc.Logout(c.Context(), req.RefreshToken)
	}
	return c.JSON(fiber.Map{"message": "logged out"})
}

// POST /auth/switch-org  { "tenant_id": "..." }  (auth required)
func (h *Handler) SwitchOrg(c fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	var req struct {
		TenantID string `json:"tenant_id"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	target, err := uuid.Parse(req.TenantID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant_id"})
	}
	ua, ip := clientMeta(c)
	bundle, err := h.svc.SwitchOrg(c.Context(), userID, target, ua, ip)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(bundle)
}

// GET /auth/me  (auth + tenant context)
func (h *Handler) Me(c fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	tenantID := middleware.CurrentTenantID(c)
	role, _ := c.Locals(middleware.LocalRole).(string)

	u, err := h.svc.q.GetUserByID(c.Context(), pgUUID(userID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}
	prof, _ := h.svc.q.GetUserProfile(c.Context(), db.GetUserProfileParams{
		TenantID: pgUUID(tenantID), UserID: pgUUID(userID),
	})
	memberships, _ := h.svc.q.ListMembershipsForUser(database.WithSuperAdmin(c.Context()), pgUUID(userID))

	orgs := make([]fiber.Map, 0, len(memberships))
	for _, m := range memberships {
		orgs = append(orgs, fiber.Map{
			"tenant_id": uuid.UUID(m.TenantID.Bytes), "org_code": m.OrgCode,
			"name": m.Name, "role": string(m.Role),
		})
	}
	return c.JSON(fiber.Map{
		"id": userID, "email": textVal(u.Email), "phone": textVal(u.Phone),
		"full_name": textVal(u.FullName), "avatar_url": textVal(u.AvatarUrl),
		"role": role, "tenant_id": tenantID,
		"is_platform_super_admin": u.IsPlatformSuperAdmin,
		"onboarding_completed":    prof.OnboardingCompleted,
		"class_level":          textVal(prof.ClassLevel),
		"board":                textVal(prof.Board),
		"exam_goal":            textVal(prof.ExamGoal),
		"orgs":                 orgs,
	})
}

// POST /auth/link/phone
func (h *Handler) LinkPhone(c fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	var req struct {
		Phone   string `json:"phone"`
		Code    string `json:"code"`
		OrgCode string `json:"org_code"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := h.svc.LinkPhone(c.Context(), userID, req.Phone, req.Code, req.OrgCode); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"linked": true})
}

// POST /auth/link/google
func (h *Handler) LinkGoogle(c fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	var req GoogleCredential
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := h.svc.LinkGoogle(c.Context(), userID, req.IDToken); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"linked": true})
}
