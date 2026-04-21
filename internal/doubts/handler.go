package doubts

import (
	"strconv"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func doubtToMap(d *db.Doubt) fiber.Map {
	return fiber.Map{
		"id":            utils.UUIDFromPg(d.ID),
		"user_id":       utils.UUIDFromPg(d.UserID),
		"lecture_id":    utils.UUIDFromPg(d.LectureID),
		"chapter_id":    utils.UUIDFromPg(d.ChapterID),
		"topic_id":      utils.UUIDFromPg(d.TopicID),
		"question_text": d.QuestionText,
		"input_type":    utils.TextFromPg(d.InputType),
		"voice_url":     utils.TextFromPg(d.VoiceUrl),
		"status":        utils.TextFromPg(d.Status),
		"language":      utils.TextFromPg(d.Language),
		"created_at":    d.CreatedAt,
	}
}

func answerToMap(a *db.DoubtAnswer) fiber.Map {
	return fiber.Map{
		"id":          utils.UUIDFromPg(a.ID),
		"doubt_id":    utils.UUIDFromPg(a.DoubtID),
		"answer_text": a.AnswerText,
		"answer_type": utils.TextFromPg(a.AnswerType),
		"answered_by": utils.UUIDFromPg(a.AnsweredBy),
		"is_accepted": utils.BoolFromPg(a.IsAccepted),
		"model_name":  utils.TextFromPg(a.ModelName),
		"created_at":  a.CreatedAt,
	}
}

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit := int32(20)
	offset := int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// Ask godoc
// @Summary Submit a doubt — optionally get an AI answer synchronously
// @Tags doubts
// @Security BearerAuth
// @Router /doubts [post]
func (h *Handler) Ask(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req AskDoubtRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	d, ai, err := h.service.Ask(c.Context(), userID, req)
	if err != nil && d == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	resp := fiber.Map{"doubt": doubtToMap(d)}
	if ai != nil {
		resp["ai_answer"] = answerToMap(ai)
	}
	if err != nil {
		resp["ai_error"] = err.Error()
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// GetDoubt godoc
// @Summary Get a doubt and all its answers
// @Tags doubts
// @Security BearerAuth
// @Router /doubts/{id} [get]
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
	for i := range answers {
		out[i] = answerToMap(&answers[i])
	}
	resp := doubtToMap(d)
	resp["answers"] = out
	return c.JSON(resp)
}

// ListMine godoc
// @Summary List my doubts
// @Tags doubts
// @Security BearerAuth
// @Router /doubts/my [get]
func (h *Handler) ListMine(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMine(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = doubtToMap(&rows[i])
	}
	return c.JSON(out)
}

// ListByLecture godoc
// @Summary List public doubts for a specific lecture
// @Tags doubts
// @Security BearerAuth
// @Router /doubts/lecture/{lecture_id} [get]
func (h *Handler) ListByLecture(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("lecture_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid lecture id"})
	}
	limit, offset := parsePagination(c)
	rows, err := h.service.ListByLecture(c.Context(), id, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = doubtToMap(&rows[i])
	}
	return c.JSON(out)
}

// ListPending godoc
// @Summary List pending doubts for instructors
// @Tags doubts
// @Security BearerAuth
// @Router /doubts/pending [get]
func (h *Handler) ListPending(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListPending(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = doubtToMap(&rows[i])
	}
	return c.JSON(out)
}

// InstructorAnswer godoc
// @Summary Post an instructor answer to a doubt
// @Tags doubts
// @Security BearerAuth
// @Router /doubts/answer [post]
func (h *Handler) InstructorAnswer(c fiber.Ctx) error {
	instrID, _ := c.Locals("userID").(uuid.UUID)
	var req InstructorAnswerRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	a, err := h.service.AnswerAsInstructor(c.Context(), instrID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(answerToMap(a))
}

// AcceptAnswer godoc
// @Summary Mark an answer as accepted
// @Tags doubts
// @Security BearerAuth
// @Router /doubts/answers/{id}/accept [post]
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

// Delete godoc
// @Summary Delete a doubt
// @Tags doubts
// @Security BearerAuth
// @Router /doubts/{id} [delete]
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
