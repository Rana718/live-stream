package enrollments

import (
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func (h *Handler) tenant(c fiber.Ctx) uuid.UUID { return middleware.CurrentTenantID(c) }
func (h *Handler) user(c fiber.Ctx) uuid.UUID   { return middleware.CurrentUserID(c) }

func pct(bps int32) float64 { return float64(bps) / 100 }
func ts(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

// Enroll — POST /enrollments
func (h *Handler) Enroll(c fiber.Ctx) error {
	var req EnrollRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	e, err := h.service.Enroll(c.Context(), h.tenant(c), h.user(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": utils.UUIDFromPg(e.ID), "user_id": utils.UUIDFromPg(e.UserID),
		"course_id": utils.UUIDFromPg(e.CourseID), "batch_id": utils.UUIDFromPg(e.BatchID),
		"status": string(e.Status), "progress_percent": pct(e.ProgressBps),
		"enrolled_at": ts(e.StartedAt),
	})
}

// ListMine — GET /enrollments/my
func (h *Handler) ListMine(c fiber.Ctx) error {
	rows, err := h.service.ListMine(c.Context(), h.tenant(c), h.user(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "course_id": utils.UUIDFromPg(r.CourseID),
			"course_title": r.Title, "course_thumbnail": utils.TextFromPg(r.ThumbnailUrl),
			"batch_id": utils.UUIDFromPg(r.BatchID), "status": string(r.Status),
			"progress_percent": pct(r.ProgressBps), "progress_bps": r.ProgressBps,
			"enrolled_at": ts(r.StartedAt), "completed_at": ts(r.CompletedAt),
		}
	}
	return c.JSON(out)
}

// ListByCourse — GET /courses/:course_id/enrollments  (roster)
func (h *Handler) ListByCourse(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	rows, err := h.service.ListRoster(c.Context(), h.tenant(c), courseID, 500, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "user_id": utils.UUIDFromPg(r.UserID),
			"course_id": courseID, "batch_id": utils.UUIDFromPg(r.BatchID),
			"full_name": utils.TextFromPg(r.FullName), "email": utils.TextFromPg(r.Email),
			"phone": utils.TextFromPg(r.Phone), "status": string(r.Status),
			"progress_percent": pct(r.ProgressBps), "enrolled_at": ts(r.StartedAt),
		}
	}
	return c.JSON(out)
}

// AdminEnroll — POST /admin/enrollments
func (h *Handler) AdminEnroll(c fiber.Ctx) error {
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
		p, perr := uuid.Parse(*body.BatchID)
		if perr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid batch id"})
		}
		bid = &p
	}
	e, err := h.service.Enroll(c.Context(), h.tenant(c), uid, EnrollRequest{CourseID: cid, BatchID: bid})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": utils.UUIDFromPg(e.ID)})
}

// Cancel — DELETE /enrollments/:course_id
func (h *Handler) Cancel(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	if err := h.service.Cancel(c.Context(), h.tenant(c), h.user(c), courseID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "cancelled"})
}
