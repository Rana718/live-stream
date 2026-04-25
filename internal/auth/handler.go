package auth

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterStudent and RegisterInstructor were the legacy email/password paths.
// They now return 410 Gone — both flows happen automatically via the OTP
// verify endpoint, which auto-creates a user the first time a phone is seen.
//
// @Summary [deprecated] Self-serve student/instructor registration
// @Tags    auth
// @Failure 410 {object} map[string]interface{}
// @Router  /auth/register/student [post]
func (h *Handler) RegisterStudent(c fiber.Ctx) error {
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error": "self-serve email registration removed",
		"hint":  "POST /auth/otp/send to start phone-OTP signup",
	})
}

// @Summary [deprecated] Self-serve instructor registration
// @Tags    auth
// @Router  /auth/register/instructor [post]
func (h *Handler) RegisterInstructor(c fiber.Ctx) error {
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error": "self-serve email registration removed",
		"hint":  "instructors are invited by tenant admins via POST /admin/users",
	})
}

// RegisterAdmin pre-creates a user record by phone for an admin role.
// Bulk-onboarding tools call this to seed the user table before the human
// completes auth via OTP. No password is set.
//
// @Summary  Pre-create an admin user record (phone-only)
// @Tags     auth
// @Security BearerAuth
// @Param    request body RegisterRequest true "Admin shell — full_name + phone + org_code"
// @Success  201 {object} map[string]interface{}
// @Router   /auth/register/admin [post]
func (h *Handler) RegisterAdmin(c fiber.Ctx) error {
	var req RegisterRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	user, err := h.service.RegisterAdmin(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "admin user shell created — they sign in via /auth/otp/verify",
		"user":    user,
	})
}

// Login godoc
// Login (deprecated): email/password login was removed. Phone OTP and Google
// sign-in are the only supported flows. Old clients hit this and get a 410
// with a hint pointing them at /auth/otp/send.
//
// @Summary  [deprecated] Email login
// @Tags     auth
// @Failure  410 {object} map[string]interface{}
// @Router   /auth/login [post]
func (h *Handler) Login(c fiber.Ctx) error {
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error":  "email login removed",
		"hint":   "use POST /auth/otp/send and POST /auth/otp/verify",
		"google": "POST /auth/google",
	})
}

// Logout godoc
// @Summary User logout
// @Description Logout and invalidate refresh token. Clears JWT cookies.
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "Logged out successfully"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Logout failed"
// @Router /auth/logout [post]
func (h *Handler) Logout(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	if err := h.service.Logout(c.Context(), userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "logout failed"})
	}

	return c.JSON(fiber.Map{"message": "logged out successfully"})
}

// RefreshToken godoc
// @Summary Refresh access token
// @Description Get new access token using refresh token from cookie or request body
// @Tags auth
// @Accept json
// @Produce json
// @Param request body map[string]string false "Refresh token (optional if using cookies)"
// @Success 200 {object} TokenResponse "Token refreshed successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Invalid refresh token"
// @Router /auth/refresh [post]
func (h *Handler) RefreshToken(c fiber.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.RefreshToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "refresh token required"})
	}

	tokens, err := h.service.RefreshToken(c.Context(), req.RefreshToken)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(tokens)
}

// GetMe godoc
// @Summary Get current user info
// @Description Get the currently authenticated user's information
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "Current user info"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Router /auth/me [get]
func (h *Handler) GetMe(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	me, err := h.service.GetMe(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}
	return c.JSON(me)
}
