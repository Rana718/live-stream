package enrollments

import (
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

// Enroll godoc
// @Summary Enroll the current user in a course
// @Tags enrollments
// @Security BearerAuth
// @Router /enrollments [post]
func (h *Handler) Enroll(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	var req EnrollRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	e, err := h.service.Enroll(c.Context(), tenantID, userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":                utils.UUIDFromPg(e.ID),
		"user_id":           utils.UUIDFromPg(e.UserID),
		"course_id":         utils.UUIDFromPg(e.CourseID),
		"batch_id":          utils.UUIDFromPg(e.BatchID),
		"status":            utils.TextFromPg(e.Status),
		"progress_percent":  utils.NumericToFloat(e.ProgressPercent),
		"enrolled_at":       e.EnrolledAt,
	})
}

// ListMine godoc
// @Summary List my enrolled courses
// @Tags enrollments
// @Security BearerAuth
// @Router /enrollments/my [get]
func (h *Handler) ListMine(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	rows, err := h.service.ListMine(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":                utils.UUIDFromPg(r.ID),
			"course_id":         utils.UUIDFromPg(r.CourseID),
			"course_title":      r.CourseTitle,
			"course_thumbnail":  utils.TextFromPg(r.CourseThumbnail),
			"batch_id":          utils.UUIDFromPg(r.BatchID),
			"status":            utils.TextFromPg(r.Status),
			"progress_percent":  utils.NumericToFloat(r.ProgressPercent),
			"enrolled_at":       r.EnrolledAt,
			"completed_at":      r.CompletedAt,
		}
	}
	return c.JSON(out)
}

// ListByCourse godoc — admin/instructor view of a course's roster.
//
// @Summary List enrollments for a course
// @Tags enrollments
// @Security BearerAuth
// @Router /courses/{course_id}/enrollments [get]
func (h *Handler) ListByCourse(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	limit := int32(500)
	rows, err := h.service.ListCourseEnrollments(c.Context(), courseID, limit, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":               utils.UUIDFromPg(r.ID),
			"user_id":          utils.UUIDFromPg(r.UserID),
			"course_id":        utils.UUIDFromPg(r.CourseID),
			"batch_id":         utils.UUIDFromPg(r.BatchID),
			"full_name":        utils.TextFromPg(r.FullName),
			"email":            utils.TextFromPg(r.Email),
			"status":           utils.TextFromPg(r.Status),
			"progress_percent": utils.NumericToFloat(r.ProgressPercent),
			"enrolled_at":      r.EnrolledAt,
		}
	}
	return c.JSON(out)
}

// AdminEnroll — admin manually enrolls a student in a course.
//
// @Summary Admin manual enrollment
// @Tags enrollments
// @Security BearerAuth
// @Router /admin/enrollments [post]
func (h *Handler) AdminEnroll(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	var body struct {
		UserID   string  `json:"user_id"`
		CourseID string  `json:"course_id"`
		BatchID  *string `json:"batch_id"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	uid, err := uuid.Parse(body.UserID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	cid, err := uuid.Parse(body.CourseID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	var bid *uuid.UUID
	if body.BatchID != nil && *body.BatchID != "" {
		parsed, perr := uuid.Parse(*body.BatchID)
		if perr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid batch id"})
		}
		bid = &parsed
	}
	e, err := h.service.Enroll(c.Context(), tenantID, uid, EnrollRequest{CourseID: cid, BatchID: bid})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": utils.UUIDFromPg(e.ID)})
}

// Cancel godoc
// @Summary Cancel my enrollment in a course
// @Tags enrollments
// @Security BearerAuth
// @Router /enrollments/{course_id} [delete]
func (h *Handler) Cancel(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	if err := h.service.Cancel(c.Context(), userID, courseID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "cancelled"})
}
