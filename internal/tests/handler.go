package tests

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

func testToMap(t *db.Test) fiber.Map {
	return fiber.Map{
		"id":                utils.UUIDFromPg(t.ID),
		"course_id":         utils.UUIDFromPg(t.CourseID),
		"subject_id":        utils.UUIDFromPg(t.SubjectID),
		"chapter_id":        utils.UUIDFromPg(t.ChapterID),
		"topic_id":          utils.UUIDFromPg(t.TopicID),
		"exam_category_id":  utils.UUIDFromPg(t.ExamCategoryID),
		"title":             t.Title,
		"description":       utils.TextFromPg(t.Description),
		"test_type":         t.TestType,
		"exam_year":         utils.Int4FromPg(t.ExamYear),
		"duration_minutes":  utils.Int4FromPg(t.DurationMinutes),
		"total_marks":       utils.NumericToFloat(t.TotalMarks),
		"passing_marks":     utils.NumericToFloat(t.PassingMarks),
		"negative_marking":  utils.BoolFromPg(t.NegativeMarking),
		"shuffle_questions": utils.BoolFromPg(t.ShuffleQuestions),
		"language":          utils.TextFromPg(t.Language),
		"is_free":           utils.BoolFromPg(t.IsFree),
		"is_published":      utils.BoolFromPg(t.IsPublished),
		"scheduled_at":      t.ScheduledAt,
		"created_at":        t.CreatedAt,
	}
}

func questionToMap(q *db.Question) fiber.Map {
	return fiber.Map{
		"id":             utils.UUIDFromPg(q.ID),
		"test_id":        utils.UUIDFromPg(q.TestID),
		"topic_id":       utils.UUIDFromPg(q.TopicID),
		"question_text":  q.QuestionText,
		"question_type":  utils.TextFromPg(q.QuestionType),
		"image_url":      utils.TextFromPg(q.ImageUrl),
		"marks":          utils.NumericToFloat(q.Marks),
		"negative_marks": utils.NumericToFloat(q.NegativeMarks),
		"difficulty":     utils.TextFromPg(q.Difficulty),
		"explanation":    utils.TextFromPg(q.Explanation),
		"display_order":  utils.Int4FromPg(q.DisplayOrder),
	}
}

func optionToMap(o *db.QuestionOption) fiber.Map {
	return fiber.Map{
		"id":            utils.UUIDFromPg(o.ID),
		"question_id":   utils.UUIDFromPg(o.QuestionID),
		"option_text":   o.OptionText,
		"image_url":     utils.TextFromPg(o.ImageUrl),
		"display_order": utils.Int4FromPg(o.DisplayOrder),
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

// CreateTest godoc
// @Summary Create a new test (instructor/admin)
// @Tags tests
// @Security BearerAuth
// @Router /tests [post]
func (h *Handler) CreateTest(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req CreateTestRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	t, err := h.service.CreateTest(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(testToMap(t))
}

// ListTests godoc
// @Summary List tests filtered by query params
// @Tags tests
// @Param type query string false "test_type (dpp|chapter|subject|mock|pyq)"
// @Param chapter_id query string false "Chapter UUID"
// @Param subject_id query string false "Subject UUID"
// @Param course_id query string false "Course UUID"
// @Param year query int false "Exam year (for PYQs)"
// @Param exam_category query string false "Exam category UUID (for PYQs)"
// @Router /tests [get]
func (h *Handler) ListTests(c fiber.Ctx) error {
	limit, offset := parsePagination(c)

	if t := c.Query("type"); t != "" {
		rows, err := h.service.ListByType(c.Context(), t, limit, offset)
		return renderTests(c, rows, err)
	}
	if v := c.Query("chapter_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid chapter_id"})
		}
		rows, err := h.service.ListByChapter(c.Context(), id)
		return renderTests(c, rows, err)
	}
	if v := c.Query("subject_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid subject_id"})
		}
		rows, err := h.service.ListBySubject(c.Context(), id)
		return renderTests(c, rows, err)
	}
	if v := c.Query("course_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course_id"})
		}
		rows, err := h.service.ListByCourse(c.Context(), id)
		return renderTests(c, rows, err)
	}
	if v := c.Query("year"); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid year"})
		}
		rows, err := h.service.ListPYQsByYear(c.Context(), int32(y))
		return renderTests(c, rows, err)
	}
	if v := c.Query("exam_category"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid exam_category"})
		}
		rows, err := h.service.ListPYQsByCategory(c.Context(), id)
		return renderTests(c, rows, err)
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "provide at least one filter"})
}

func renderTests(c fiber.Ctx, rows []db.Test, err error) error {
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = testToMap(&rows[i])
	}
	return c.JSON(out)
}

// GetTest godoc
// @Summary Get a test with its questions
// @Tags tests
// @Router /tests/{id} [get]
func (h *Handler) GetTest(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	t, err := h.service.GetTest(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	questions, err := h.service.ListQuestions(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	qs := make([]fiber.Map, len(questions))
	for i := range questions {
		qs[i] = questionToMap(&questions[i])
		opts, _ := h.service.ListOptions(c.Context(), uuid.UUID(questions[i].ID.Bytes))
		mapped := make([]fiber.Map, len(opts))
		for j := range opts {
			mapped[j] = optionToMap(&opts[j])
		}
		qs[i]["options"] = mapped
	}
	resp := testToMap(t)
	resp["questions"] = qs
	return c.JSON(resp)
}

// UpdateTest godoc
// @Tags tests
// @Security BearerAuth
// @Router /tests/{id} [put]
func (h *Handler) UpdateTest(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req CreateTestRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	t, err := h.service.UpdateTest(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(testToMap(t))
}

// DeleteTest godoc
// @Tags tests
// @Security BearerAuth
// @Router /tests/{id} [delete]
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

// CreateQuestion godoc
// @Summary Add a question (with options) to a test
// @Tags tests
// @Security BearerAuth
// @Router /tests/questions [post]
func (h *Handler) CreateQuestion(c fiber.Ctx) error {
	var req CreateQuestionRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	q, opts, err := h.service.CreateQuestion(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	mappedOpts := make([]fiber.Map, len(opts))
	for i := range opts {
		mappedOpts[i] = optionToMap(&opts[i])
	}
	resp := questionToMap(q)
	resp["options"] = mappedOpts
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// DeleteQuestion godoc
// @Tags tests
// @Security BearerAuth
// @Router /tests/questions/{id} [delete]
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

// StartAttempt godoc
// @Summary Start a test attempt for the current user
// @Tags tests
// @Security BearerAuth
// @Router /tests/{id}/attempts [post]
func (h *Handler) StartAttempt(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	testID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid test id"})
	}
	att, err := h.service.StartAttempt(c.Context(), userID, testID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(attemptToMap(att))
}

// SubmitAnswer godoc
// @Summary Submit an answer during an attempt
// @Tags tests
// @Security BearerAuth
// @Router /tests/attempts/answer [post]
func (h *Handler) SubmitAnswer(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req SubmitAnswerRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	ans, err := h.service.SubmitAnswer(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"id":                 utils.UUIDFromPg(ans.ID),
		"is_correct":         utils.BoolFromPg(ans.IsCorrect),
		"marks_obtained":     utils.NumericToFloat(ans.MarksObtained),
		"time_taken_seconds": utils.Int4FromPg(ans.TimeTakenSeconds),
	})
}

// SubmitAttempt godoc
// @Summary Submit the current attempt and compute final score
// @Tags tests
// @Security BearerAuth
// @Router /tests/attempts/{id}/submit [post]
func (h *Handler) SubmitAttempt(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid attempt id"})
	}
	att, err := h.service.SubmitAttempt(c.Context(), userID, id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(attemptToMap(att))
}

// GetAttempt godoc
// @Summary Get details of an attempt with its answers
// @Tags tests
// @Security BearerAuth
// @Router /tests/attempts/{id} [get]
func (h *Handler) GetAttempt(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid attempt id"})
	}
	att, err := h.service.GetAttempt(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	answers, _ := h.service.ListAttemptAnswers(c.Context(), id)
	mapped := make([]fiber.Map, len(answers))
	for i, a := range answers {
		mapped[i] = fiber.Map{
			"question_id":        utils.UUIDFromPg(a.QuestionID),
			"selected_option_id": utils.UUIDFromPg(a.SelectedOptionID),
			"numerical_answer":   utils.NumericToFloat(a.NumericalAnswer),
			"subjective_answer":  utils.TextFromPg(a.SubjectiveAnswer),
			"is_correct":         utils.BoolFromPg(a.IsCorrect),
			"marks_obtained":     utils.NumericToFloat(a.MarksObtained),
		}
	}
	resp := attemptToMap(att)
	resp["answers"] = mapped
	return c.JSON(resp)
}

// ListMyAttempts godoc
// @Summary List my test attempts
// @Tags tests
// @Security BearerAuth
// @Router /tests/attempts/my [get]
func (h *Handler) ListMyAttempts(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMyAttempts(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":                 utils.UUIDFromPg(r.ID),
			"test_id":            utils.UUIDFromPg(r.TestID),
			"test_title":         r.TestTitle,
			"test_type":          r.TestType,
			"status":             utils.TextFromPg(r.Status),
			"score":              utils.NumericToFloat(r.Score),
			"correct_count":      utils.Int4FromPg(r.CorrectCount),
			"wrong_count":        utils.Int4FromPg(r.WrongCount),
			"time_taken_seconds": utils.Int4FromPg(r.TimeTakenSeconds),
			"started_at":         r.StartedAt,
			"submitted_at":       r.SubmittedAt,
		}
	}
	return c.JSON(out)
}

func attemptToMap(a *db.TestAttempt) fiber.Map {
	return fiber.Map{
		"id":                 utils.UUIDFromPg(a.ID),
		"user_id":            utils.UUIDFromPg(a.UserID),
		"test_id":            utils.UUIDFromPg(a.TestID),
		"status":             utils.TextFromPg(a.Status),
		"score":              utils.NumericToFloat(a.Score),
		"total_questions":    utils.Int4FromPg(a.TotalQuestions),
		"correct_count":      utils.Int4FromPg(a.CorrectCount),
		"wrong_count":        utils.Int4FromPg(a.WrongCount),
		"skipped_count":      utils.Int4FromPg(a.SkippedCount),
		"time_taken_seconds": utils.Int4FromPg(a.TimeTakenSeconds),
		"started_at":         a.StartedAt,
		"submitted_at":       a.SubmittedAt,
	}
}
