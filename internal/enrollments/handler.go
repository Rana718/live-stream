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
	var req EnrollRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	e, err := h.service.Enroll(c.Context(), userID, req)
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
