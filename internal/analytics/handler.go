package analytics

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

// GetMyStats godoc
// @Summary Get aggregate stats for the current user
// @Tags analytics
// @Security BearerAuth
// @Router /analytics/me [get]
func (h *Handler) GetMyStats(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	stats, err := h.service.GetUserStats(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(stats)
}

// GetWeakTopics godoc
// @Summary Get topics where the user is weakest
// @Tags analytics
// @Security BearerAuth
// @Router /analytics/weak-topics [get]
func (h *Handler) GetWeakTopics(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	limit := 10
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	topics, err := h.service.GetWeakTopics(c.Context(), userID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(topics)
}

// GetDifficultyBreakdown godoc
// @Summary Get accuracy broken down by question difficulty
// @Tags analytics
// @Security BearerAuth
// @Router /analytics/difficulty [get]
func (h *Handler) GetDifficultyBreakdown(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	rows, err := h.service.GetDifficultyBreakdown(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// TenantDashboard — GET /api/v1/analytics/tenant/dashboard
//
// Tenant_admin-only. Returns the headline stats card payload + a 30-day
// revenue series + top courses by enrollment, all RLS-scoped to the
// caller's tenant.
//
//	@Summary  Tenant admin dashboard stats
//	@Tags     analytics
//	@Security BearerAuth
//	@Router   /analytics/tenant/dashboard [get]
func (h *Handler) TenantDashboard(c fiber.Ctx) error {
	stats, err := h.service.TenantDashboard(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	revenue, _ := h.service.TenantRevenueDaily(c.Context())
	top, _ := h.service.TenantTopCourses(c.Context(), 5)
	return c.JSON(fiber.Map{
		"stats":         stats,
		"revenue_daily": revenue,
		"top_courses":   top,
	})
}

// GetRecentAttempts godoc
// @Summary List recent test attempts with scores
// @Tags analytics
// @Security BearerAuth
// @Router /analytics/recent-attempts [get]
func (h *Handler) GetRecentAttempts(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	limit := int32(10)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = int32(l)
	}
	rows, err := h.service.GetRecentAttempts(c.Context(), userID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}
