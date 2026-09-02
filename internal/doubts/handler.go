package doubts

import (
	"strconv"

	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func answerView(id, doubtID, answeredBy pgtype.UUID, text, atype string, accepted bool, model string, createdAt any) fiber.Map {
	return fiber.Map{
		"id":          utils.UUIDFromPg(id),
		"doubt_id":    utils.UUIDFromPg(doubtID),
		"answer_text": text,
		"content":     text, // legacy alias
		"answer_type": atype,
		"answered_by": utils.UUIDFromPg(answeredBy),
		"is_accepted": accepted,
		"accepted":    accepted,
		"model_name":  model,
		"created_at":  createdAt,
	}
}

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit, offset := int32(20), int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// Ask — POST /doubts
func (h *Handler) Ask(c fiber.Ctx) error {
	var req AskDoubtRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if len(req.question()) < 3 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "question is required"})
	}
	d, ai, err := h.service.Ask(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), req)
	if err != nil && d.ID.Valid == false {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	resp := fiber.Map{
		"doubt": fiber.Map{
			"id":            utils.UUIDFromPg(d.ID),
			"user_id":       utils.UUIDFromPg(d.UserID),
			"question":      d.QuestionText,
			"question_text": d.QuestionText,
			"status":        string(d.Status),
			"created_at":    d.CreatedAt.Time,
		},
	}
	if ai != nil {
		resp["ai_answer"] = answerView(ai.ID, ai.DoubtID, ai.AnsweredBy, ai.AnswerText, ai.AnswerType, ai.IsAccepted, "", ai.CreatedAt.Time)
	}
	if err != nil {
		resp["ai_error"] = err.Error()
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// Get — GET /doubts/:id
func (h *Handler) Get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	d, answers, err := h.service.Get(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	out := make([]fiber.Map, len(answers))
	for i, a := range answers {
		out[i] = answerView(a.ID, utils.UUIDToPg(id), a.AnsweredBy, a.AnswerText, a.AnswerType, a.IsAccepted, utils.TextFromPg(a.ModelName), a.CreatedAt.Time)
	}
	return c.JSON(fiber.Map{
		"id":            utils.UUIDFromPg(d.ID),
		"user_id":       utils.UUIDFromPg(d.UserID),
		"lesson_id":     utils.UUIDFromPg(d.LessonID),
		"chapter_id":    utils.UUIDFromPg(d.ChapterID),
		"topic_id":      utils.UUIDFromPg(d.TopicID),
		"question":      d.QuestionText,
		"question_text": d.QuestionText,
		"input_type":     d.InputType,
		"attachment_url": utils.TextFromPg(d.AttachmentUrl),
		"status":         string(d.Status),
		"language":       d.Language,
		"created_at":    d.CreatedAt.Time,
		"answers":       out,
	})
}

// ListMine — GET /doubts/my
func (h *Handler) ListMine(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMine(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":            utils.UUIDFromPg(r.ID),
			"question":      r.QuestionText,
			"question_text": r.QuestionText,
			"status":        string(r.Status),
			"created_at":    r.CreatedAt.Time,
		}
	}
	return c.JSON(out)
}

// ListByLecture — GET /doubts/lecture/:lecture_id. Not scoped server-side in
// v2; returns pending for the tenant. (Retired in Phase J.)
func (h *Handler) ListByLecture(c fiber.Ctx) error {
	return h.ListPending(c)
}

// ListPending — GET /doubts/pending  (instructor/admin)
func (h *Handler) ListPending(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListPending(c.Context(), middleware.CurrentTenantID(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":            utils.UUIDFromPg(r.ID),
			"user_id":       utils.UUIDFromPg(r.UserID),
			"student_name":  utils.TextFromPg(r.FullName),
			"question":      r.QuestionText,
			"question_text": r.QuestionText,
			"created_at":    r.CreatedAt.Time,
		}
	}
	return c.JSON(out)
}

// InstructorAnswer — POST /doubts/answer
func (h *Handler) InstructorAnswer(c fiber.Ctx) error {
	var req InstructorAnswerRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if len(req.text()) < 3 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "answer is required"})
	}
	a, err := h.service.AnswerAsInstructor(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(answerView(a.ID, a.DoubtID, a.AnsweredBy, a.AnswerText, a.AnswerType, a.IsAccepted, "", a.CreatedAt.Time))
}

// AcceptAnswer — POST /doubts/answers/:id/accept
func (h *Handler) AcceptAnswer(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Accept(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "accepted"})
}

// Delete — DELETE /doubts/:id. No hard-delete in v2; mark closed.
func (h *Handler) Delete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.SetStatus(c.Context(), id, "closed"); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "closed"})
}
