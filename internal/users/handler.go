package users

import (
	"strconv"
	
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetProfile godoc
// @Summary Get user profile
// @Description Get the profile of the authenticated user
// @Tags users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} UserProfile "User profile"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 404 {object} map[string]interface{} "User not found"
// @Router /users/profile [get]
func (h *Handler) GetProfile(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	profile, err := h.service.GetUserProfile(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	return c.JSON(profile)
}

// UpdateProfile godoc
// @Summary Update user profile
// @Description Update the profile of the authenticated user
// @Tags users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]string true "Profile update data"
// @Success 200 {object} map[string]interface{} "Profile updated"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Router /users/profile [put]
func (h *Handler) UpdateProfile(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	var req struct {
		FullName string `json:"full_name"`
	}

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	user, err := h.service.UpdateUser(c.Context(), userID, req.FullName)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(user)
}

// CompleteOnboarding sets the learner's class_level / board / exam_goal and
// flips onboarding_completed so the mobile router stops redirecting them
// back to the onboarding screen.
// @Router /users/me/onboarding [post]
func (h *Handler) CompleteOnboarding(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	var req struct {
		FullName   string `json:"full_name"`
		ClassLevel string `json:"class_level"`
		Board      string `json:"board"`
		ExamGoal   string `json:"exam_goal"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.ClassLevel == "" && req.ExamGoal == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "pick at least one of class_level or exam_goal",
		})
	}

	if _, err := h.service.CompleteOnboarding(c.Context(), userID, OnboardingInput{
		FullName:   req.FullName,
		ClassLevel: req.ClassLevel,
		Board:      req.Board,
		ExamGoal:   req.ExamGoal,
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	profile, err := h.service.GetUserProfile(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(profile)
}

// ListUsers godoc
// @Summary List all users
// @Description Get a list of all users (Admin only)
// @Tags users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param limit query int false "Limit" default(10)
// @Param offset query int false "Offset" default(0)
// @Success 200 {array} map[string]interface{} "List of users"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 403 {object} map[string]interface{} "Forbidden"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /users [get]
func (h *Handler) ListUsers(c fiber.Ctx) error {
	limit := 10
	offset := 0
	
	if l := c.Query("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}
	
	if o := c.Query("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil {
			offset = val
		}
	}

	users, err := h.service.ListUsers(c.Context(), int32(limit), int32(offset))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(users)
}
