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

// RegisterStudent godoc
// @Summary Register a new student
// @Description Register a new student account
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Student registration details"
// @Success 201 {object} map[string]interface{} "Student registered successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Router /auth/register/student [post]
func (h *Handler) RegisterStudent(c fiber.Ctx) error {
	var req RegisterRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	user, err := h.service.RegisterStudent(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "student registered successfully",
		"user":    user,
	})
}

// RegisterInstructor godoc
// @Summary Register a new instructor
// @Description Register a new instructor account (requires admin approval in production)
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Instructor registration details"
// @Success 201 {object} map[string]interface{} "Instructor registered successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Router /auth/register/instructor [post]
func (h *Handler) RegisterInstructor(c fiber.Ctx) error {
	var req RegisterRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	user, err := h.service.RegisterInstructor(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "instructor registered successfully",
		"user":    user,
	})
}

// RegisterAdmin godoc
// @Summary Register a new admin
// @Description Register a new admin account (protected - requires existing admin)
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body RegisterRequest true "Admin registration details"
// @Success 201 {object} map[string]interface{} "Admin registered successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 403 {object} map[string]interface{} "Forbidden"
// @Router /auth/register/admin [post]
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
		"message": "admin registered successfully",
		"user":    user,
	})
}

// Login godoc
// @Summary User login
// @Description Login with email and password. Sets JWT tokens in HTTP-only cookies.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login credentials"
// @Success 200 {object} TokenResponse "Login successful"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Invalid credentials"
// @Router /auth/login [post]
func (h *Handler) Login(c fiber.Ctx) error {
	var req LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	tokens, err := h.service.Login(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(tokens)
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
