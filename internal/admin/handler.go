package admin

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit := int32(50)
	offset := int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// Dashboard godoc
// @Summary Admin dashboard — aggregate stats across the platform
// @Tags admin
// @Security BearerAuth
// @Router /admin/dashboard [get]
func (h *Handler) Dashboard(c fiber.Ctx) error {
	stats, err := h.service.DashboardStats(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(stats)
}

// ListUsers godoc
// @Summary Admin: list all users (optionally filtered by role)
// @Tags admin
// @Param role query string false "student|instructor|admin"
// @Security BearerAuth
// @Router /admin/users [get]
func (h *Handler) ListUsers(c fiber.Ctx) error {
	role := c.Query("role")
	limit, offset := parsePagination(c)
	rows, err := h.service.ListAllUsers(c.Context(), role, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":                utils.UUIDFromPg(r.ID),
			"email":             r.Email,
			"username":          r.Username,
			"full_name":         utils.TextFromPg(r.FullName),
			"role":              utils.TextFromPg(r.Role),
			"is_active":         utils.BoolFromPg(r.IsActive),
			"created_at":        r.CreatedAt,
			"enrolled_courses":  r.EnrolledCourses,
		}
	}
	return c.JSON(out)
}

// BatchAttendance godoc
// @Summary Admin: aggregate attendance percent per batch
// @Tags admin
// @Security BearerAuth
// @Router /admin/attendance/batches [get]
func (h *Handler) BatchAttendance(c fiber.Ctx) error {
	rows, err := h.service.BatchAttendance(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// ListPendingApproval godoc
// @Summary Admin: courses awaiting approval
// @Tags admin
// @Security BearerAuth
// @Router /admin/courses/pending [get]
func (h *Handler) ListPendingApproval(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListPendingApproval(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":              utils.UUIDFromPg(r.ID),
			"title":           r.Title,
			"slug":            r.Slug,
			"description":     utils.TextFromPg(r.Description),
			"created_by":      utils.UUIDFromPg(r.CreatedBy),
			"approval_status": utils.TextFromPg(r.ApprovalStatus),
			"created_at":      r.CreatedAt,
		}
	}
	return c.JSON(out)
}

// ApproveCourse godoc
// @Summary Admin: approve a course (publishes it)
// @Tags admin
// @Security BearerAuth
// @Router /admin/courses/{id}/approve [post]
func (h *Handler) ApproveCourse(c fiber.Ctx) error {
	adminID, _ := c.Locals("userID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	course, err := h.service.ApproveCourse(c.Context(), id, adminID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"id":              utils.UUIDFromPg(course.ID),
		"approval_status": utils.TextFromPg(course.ApprovalStatus),
		"is_published":    utils.BoolFromPg(course.IsPublished),
		"approved_at":     course.ApprovedAt,
	})
}

// RejectCourse godoc
// @Summary Admin: reject a course with a reason
// @Tags admin
// @Security BearerAuth
// @Router /admin/courses/{id}/reject [post]
func (h *Handler) RejectCourse(c fiber.Ctx) error {
	adminID, _ := c.Locals("userID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Reason string `json:"reason" validate:"required,min=3"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	course, err := h.service.RejectCourse(c.Context(), id, adminID, req.Reason)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"id":               utils.UUIDFromPg(course.ID),
		"approval_status":  utils.TextFromPg(course.ApprovalStatus),
		"rejection_reason": utils.TextFromPg(course.RejectionReason),
	})
}

// ExportUsersCSV godoc
// @Summary Admin: export all users as CSV
// @Tags admin
// @Security BearerAuth
// @Router /admin/users/export [get]
func (h *Handler) ExportUsersCSV(c fiber.Ctx) error {
	role := c.Query("role")
	rows, err := h.service.ListAllUsers(c.Context(), role, 10000, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="users_%s.csv"`, time.Now().Format("2006-01-02")))
	w := csv.NewWriter(c.Response().BodyWriter())
	defer w.Flush()
	_ = w.Write([]string{"id", "email", "username", "full_name", "role", "is_active", "enrolled_courses", "created_at"})
	for _, r := range rows {
		created := ""
		if r.CreatedAt.Valid {
			created = r.CreatedAt.Time.Format(time.RFC3339)
		}
		active := "false"
		if utils.BoolFromPg(r.IsActive) {
			active = "true"
		}
		_ = w.Write([]string{
			utils.UUIDFromPg(r.ID),
			r.Email,
			r.Username,
			utils.TextFromPg(r.FullName),
			utils.TextFromPg(r.Role),
			active,
			strconv.FormatInt(r.EnrolledCourses, 10),
			created,
		})
	}
	return nil
}
