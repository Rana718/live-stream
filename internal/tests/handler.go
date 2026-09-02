package tests

import (
	"strconv"

	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

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
func (h *Handler) user(c fiber.Ctx) uuid.UUID   { return middleware.CurrentUserID(c) }

// CreateTest — POST /tests  (instructor/admin)
func (h *Handler) CreateTest(c fiber.Ctx) error {
	var req CreateTestRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	t, err := h.service.CreateTest(c.Context(), h.tenant(c), h.user(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": utils.UUIDFromPg(t.ID), "title": t.Title, "kind": string(t.Kind),
		"status": string(t.Status), "total_marks": utils.NumericToFloat(t.TotalMarks),
	})
}

// ListTests — GET /tests?course_id=…
func (h *Handler) ListTests(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	var courseID *uuid.UUID
	if v := c.Query("course_id"); v != "" {
		if id, e := uuid.Parse(v); e == nil {
			courseID = &id
		}
	}
	rows, err := h.service.ListTests(c.Context(), h.tenant(c), courseID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "title": r.Title,
			"kind": string(r.Kind), "type": string(r.Kind), "test_type": string(r.Kind),
			"duration_minutes": r.DurationMin, "total_marks": utils.NumericToFloat(r.TotalMarks),
			"is_free": r.IsFree, "status": string(r.Status),
			"is_published": string(r.Status) == "published",
		}
	}
	return c.JSON(out)
}

// GetTest — GET /tests/:id  (with questions)
func (h *Handler) GetTest(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	v, err := h.service.GetTest(c.Context(), id, true)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(v)
}

// UpdateTest — PUT /tests/:id
func (h *Handler) UpdateTest(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req CreateTestRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := h.service.UpdateTest(c.Context(), id, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	v, _ := h.service.GetTest(c.Context(), id, false)
	return c.JSON(v)
}

// AdminSetPublished — PATCH /tests/:id/publish  { is_published }
func (h *Handler) AdminSetPublished(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		IsPublished bool `json:"is_published"`
	}
	_ = c.Bind().JSON(&body)
	if err := h.service.SetPublished(c.Context(), id, body.IsPublished); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"id": id, "is_published": body.IsPublished})
}

// DeleteTest — DELETE /tests/:id
func (h *Handler) DeleteTest(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.DeleteTest(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// CreateQuestion — POST /tests/questions
func (h *Handler) CreateQuestion(c fiber.Ctx) error {
	var req CreateQuestionRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.QuestionText == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "question_text required"})
	}
	q, err := h.service.CreateQuestion(c.Context(), h.tenant(c), h.user(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(q)
}

// DeleteQuestion — DELETE /tests/questions/:id
func (h *Handler) DeleteQuestion(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.DeleteQuestion(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// StartAttempt — POST /tests/:id/attempts
func (h *Handler) StartAttempt(c fiber.Ctx) error {
	testID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	a, err := h.service.StartAttempt(c.Context(), h.tenant(c), h.user(c), testID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(a)
}

// SubmitAnswer — POST /tests/attempts/answer
func (h *Handler) SubmitAnswer(c fiber.Ctx) error {
	var req SubmitAnswerRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.AttemptID == uuid.Nil || req.QuestionID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "attempt_id and question_id required"})
	}
	if err := h.service.SubmitAnswer(c.Context(), h.tenant(c), h.user(c), req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "ok"})
}

// SubmitAttempt — POST /tests/attempts/:id/submit
func (h *Handler) SubmitAttempt(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	res, err := h.service.SubmitAttempt(c.Context(), h.user(c), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

// GetAttempt — GET /tests/attempts/:id
func (h *Handler) GetAttempt(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	a, err := h.service.GetAttempt(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(a)
}

// ListMyAttempts — GET /tests/attempts/my
func (h *Handler) ListMyAttempts(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMyAttempts(c.Context(), h.tenant(c), h.user(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}
