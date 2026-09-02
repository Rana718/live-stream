package admin

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit, offset := int32(50), int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

func (h *Handler) tenant(c fiber.Ctx) uuid.UUID { return middleware.CurrentTenantID(c) }
func (h *Handler) user(c fiber.Ctx) uuid.UUID   { return middleware.CurrentUserID(c) }

// Dashboard — GET /admin/dashboard
func (h *Handler) Dashboard(c fiber.Ctx) error {
	stats, err := h.service.DashboardStats(c.Context(), h.tenant(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"total_courses":          stats.TotalCourses,
		"published_courses":      stats.PublishedCourses,
		"total_students":         stats.TotalStudents,
		"total_instructors":      stats.TotalInstructors,
		"total_users":            stats.TotalStudents + stats.TotalInstructors,
		"active_enrollments":     stats.TotalEnrollments,
		"total_enrollments":      stats.TotalEnrollments,
		"revenue_minor":          stats.RevenueMinor,
		"paid_orders":            stats.PaidOrders,
		"total_revenue_captured": float64(stats.RevenueMinor) / 100,
	})
}

func tsAny(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

// ListUsers — GET /admin/users?role=&q=
func (h *Handler) ListUsers(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMembers(c.Context(), h.tenant(c), c.Query("role"), c.Query("q"), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":                utils.UUIDFromPg(r.ID),
			"email":             utils.TextFromPg(r.Email),
			"phone":             utils.TextFromPg(r.Phone),
			"full_name":         utils.TextFromPg(r.FullName),
			"role":              string(r.Role),
			"status":            r.Status,
			"membership_status": string(r.MembershipStatus),
			"is_active":         r.Status == "active" && string(r.MembershipStatus) == "active",
			"created_at":        tsAny(r.CreatedAt),
		}
	}
	return c.JSON(out)
}

// BatchAttendance — GET /admin/attendance/batches
func (h *Handler) BatchAttendance(c fiber.Ctx) error {
	rows, err := h.service.BatchAttendance(c.Context(), h.tenant(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"batch_id": utils.UUIDFromPg(r.BatchID), "total": r.Total,
			"attended": r.Attended, "attendance_percent": r.AttendancePercent,
		}
	}
	return c.JSON(out)
}

// ListPendingApproval — GET /admin/courses/pending
func (h *Handler) ListPendingApproval(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListPendingApproval(c.Context(), h.tenant(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "title": r.Title, "slug": r.Slug,
			"created_by": utils.UUIDFromPg(r.CreatedBy), "created_at": tsAny(r.CreatedAt),
			"approval_status": "pending",
		}
	}
	return c.JSON(out)
}

// ApproveCourse — POST /admin/courses/:id/approve
func (h *Handler) ApproveCourse(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	course, err := h.service.ApproveCourse(c.Context(), id, h.user(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"id": utils.UUIDFromPg(course.ID), "approval_status": course.ApprovalStatus,
		"status": string(course.Status), "is_published": course.Status == "published",
		"approved_at": tsAny(course.ApprovedAt),
	})
}

// RejectCourse — POST /admin/courses/:id/reject
func (h *Handler) RejectCourse(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.Bind().JSON(&req)
	course, err := h.service.RejectCourse(c.Context(), id, req.Reason)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"id": utils.UUIDFromPg(course.ID), "approval_status": course.ApprovalStatus,
		"rejection_reason": utils.TextFromPg(course.RejectionReason),
	})
}

// UpdateUser — PUT /admin/users/:id
func (h *Handler) UpdateUser(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req AdminUpdateUserRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	u, err := h.service.UpdateUser(c.Context(), h.tenant(c), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"id": utils.UUIDFromPg(u.ID), "email": utils.TextFromPg(u.Email),
		"phone": utils.TextFromPg(u.Phone), "full_name": utils.TextFromPg(u.FullName),
		"role": string(u.Role),
	})
}

// SetUserRole — POST /admin/users/:id/role
func (h *Handler) SetUserRole(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := c.Bind().JSON(&req); err != nil || req.Role == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role required"})
	}
	if err := h.service.SetUserRole(c.Context(), h.tenant(c), id, req.Role); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"id": id, "role": req.Role})
}

// SetUserActive — POST /admin/users/:id/active
func (h *Handler) SetUserActive(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Active bool `json:"active"`
	}
	_ = c.Bind().JSON(&req)
	if err := h.service.SetUserActive(c.Context(), h.tenant(c), id, req.Active); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"id": id, "is_active": req.Active})
}

// ResetUserPassword — POST /admin/users/:id/password
func (h *Handler) ResetUserPassword(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := c.Bind().JSON(&req); err != nil || len(req.NewPassword) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "new_password (min 8) required"})
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "hash failed"})
	}
	if err := h.service.ResetUserPassword(c.Context(), id, string(hash)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "password reset"})
}

// DeleteUser — DELETE /admin/users/:id  (removes tenant membership)
func (h *Handler) DeleteUser(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.DeleteUser(c.Context(), h.tenant(c), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "removed from this institute"})
}

// ExportUsersCSV — GET /admin/users/export
func (h *Handler) ExportUsersCSV(c fiber.Ctx) error {
	rows, err := h.service.ListMembers(c.Context(), h.tenant(c), c.Query("role"), "", 10000, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="users_%s.csv"`, time.Now().Format("2006-01-02")))
	w := csv.NewWriter(c.Response().BodyWriter())
	defer w.Flush()
	_ = w.Write([]string{"id", "email", "phone", "full_name", "role", "status"})
	for _, r := range rows {
		_ = w.Write([]string{
			utils.UUIDFromPg(r.ID), utils.TextFromPg(r.Email), utils.TextFromPg(r.Phone),
			utils.TextFromPg(r.FullName), string(r.Role), string(r.MembershipStatus),
		})
	}
	return nil
}
