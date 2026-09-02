package lectures

import (
	"strconv"

	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func tval(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit, offset := int32(50), int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 200 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

func (h *Handler) tenant(c fiber.Ctx) uuid.UUID { return middleware.CurrentTenantID(c) }

// Create — POST /lectures  (instructor/admin). Creates a course_lesson.
func (h *Handler) Create(c fiber.Ctx) error {
	var req CreateLectureRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	l, err := h.service.Create(c.Context(), h.tenant(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(l)
}

// List — GET /lectures?course_id=…  (topic/chapter/subject/live/q → empty in v2)
func (h *Handler) List(c fiber.Ctx) error {
	if cid := c.Query("course_id"); cid != "" {
		id, err := uuid.Parse(cid)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course_id"})
		}
		rows, err := h.service.ListByCourse(c.Context(), id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(rows)
	}
	// No course filter → flat tenant-wide list. topic_id / chapter_id /
	// subject_id / live / q filters are not modelled in v2; they fall
	// through to the same list (the frontend groups client-side).
	rows, err := h.service.ListForTenant(c.Context(), h.tenant(c), nil, 200, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// Get — GET /lectures/:id
func (h *Handler) Get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	l, err := h.service.Get(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(l)
}

// Update — PUT /lectures/:id
func (h *Handler) Update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req CreateLectureRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := h.service.Update(c.Context(), id, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	l, _ := h.service.Get(c.Context(), id)
	return c.JSON(l)
}

// Delete — DELETE /lectures/:id  (soft)
func (h *Handler) Delete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Delete(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// ── course sections ─────────────────────────────────────────────────

// ListSections — GET /courses/:course_id/sections
func (h *Handler) ListSections(c fiber.Ctx) error {
	cid, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	rows, err := h.service.ListSections(c.Context(), cid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// CreateSection — POST /course-sections
func (h *Handler) CreateSection(c fiber.Ctx) error {
	var req SectionRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	s, err := h.service.CreateSection(c.Context(), h.tenant(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(s)
}

// UpdateSection — PUT /course-sections/:id
func (h *Handler) UpdateSection(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req SectionRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := h.service.UpdateSection(c.Context(), id, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"updated": true})
}

// DeleteSection — DELETE /course-sections/:id
func (h *Handler) DeleteSection(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.DeleteSection(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// RecordWatch — POST /lectures/watch
func (h *Handler) RecordWatch(c fiber.Ctx) error {
	var req RecordWatchRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := h.service.RecordWatch(c.Context(), h.tenant(c), middleware.CurrentUserID(c), req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "ok"})
}

// History — GET /lectures/history/my
func (h *Handler) History(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListHistory(c.Context(), h.tenant(c), middleware.CurrentUserID(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"lesson_id":       utils.UUIDFromPg(r.LessonID),
			"lecture_id":      utils.UUIDFromPg(r.LessonID),
			"watched_seconds": r.WatchedSec,
			"position_seconds": r.PositionSec,
			"completed":       r.CompletedAt.Valid,
			"last_watched_at": tval(r.LastAt),
		}
	}
	return c.JSON(out)
}
