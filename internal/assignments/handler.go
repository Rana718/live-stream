package assignments

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

func assignmentToMap(a *db.Assignment) fiber.Map {
	return fiber.Map{
		"id":             utils.UUIDFromPg(a.ID),
		"batch_id":       utils.UUIDFromPg(a.BatchID),
		"course_id":      utils.UUIDFromPg(a.CourseID),
		"chapter_id":     utils.UUIDFromPg(a.ChapterID),
		"topic_id":       utils.UUIDFromPg(a.TopicID),
		"title":          a.Title,
		"description":    utils.TextFromPg(a.Description),
		"attachment_url": utils.TextFromPg(a.AttachmentUrl),
		"due_date":       a.DueDate,
		"max_marks":      utils.NumericToFloat(a.MaxMarks),
		"is_published":   utils.BoolFromPg(a.IsPublished),
		"created_by":     utils.UUIDFromPg(a.CreatedBy),
		"created_at":     a.CreatedAt,
	}
}

func submissionToMap(s *db.AssignmentSubmission) fiber.Map {
	return fiber.Map{
		"id":              utils.UUIDFromPg(s.ID),
		"assignment_id":   utils.UUIDFromPg(s.AssignmentID),
		"user_id":         utils.UUIDFromPg(s.UserID),
		"submission_text": utils.TextFromPg(s.SubmissionText),
		"file_path":       utils.TextFromPg(s.FilePath),
		"submitted_at":    s.SubmittedAt,
		"graded_at":       s.GradedAt,
		"marks_obtained":  utils.NumericToFloat(s.MarksObtained),
		"feedback":        utils.TextFromPg(s.Feedback),
		"graded_by":       utils.UUIDFromPg(s.GradedBy),
		"status":          utils.TextFromPg(s.Status),
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

// Create godoc
// @Summary Create an assignment (instructor/admin)
// @Tags assignments
// @Security BearerAuth
// @Router /assignments [post]
func (h *Handler) Create(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req CreateAssignmentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	a, err := h.service.Create(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(assignmentToMap(a))
}

// Get godoc
// @Summary Get an assignment
// @Tags assignments
// @Router /assignments/{id} [get]
func (h *Handler) Get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	a, err := h.service.Get(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(assignmentToMap(a))
}

// ListByBatch godoc
// @Summary List assignments for a batch
// @Tags assignments
// @Router /assignments/batch/{batch_id} [get]
func (h *Handler) ListByBatch(c fiber.Ctx) error {
	batchID, err := uuid.Parse(c.Params("batch_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid batch id"})
	}
	limit, offset := parsePagination(c)
	rows, err := h.service.ListByBatch(c.Context(), batchID, limit, offset)
	return renderList(c, rows, err)
}

// ListByCourse godoc
// @Summary List assignments for a course
// @Tags assignments
// @Router /assignments/course/{course_id} [get]
func (h *Handler) ListByCourse(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	limit, offset := parsePagination(c)
	rows, err := h.service.ListByCourse(c.Context(), courseID, limit, offset)
	return renderList(c, rows, err)
}

// ListMine godoc
// @Summary List assignments I created (instructor)
// @Tags assignments
// @Security BearerAuth
// @Router /assignments/mine [get]
func (h *Handler) ListMine(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMyCreated(c.Context(), userID, limit, offset)
	return renderList(c, rows, err)
}

func renderList(c fiber.Ctx, rows []db.Assignment, err error) error {
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = assignmentToMap(&rows[i])
	}
	return c.JSON(out)
}

// Update godoc
// @Summary Update assignment (instructor/admin)
// @Tags assignments
// @Security BearerAuth
// @Router /assignments/{id} [put]
func (h *Handler) Update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req CreateAssignmentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	a, err := h.service.Update(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(assignmentToMap(a))
}

// Delete godoc
// @Summary Delete assignment
// @Tags assignments
// @Security BearerAuth
// @Router /assignments/{id} [delete]
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

// Submit godoc
// @Summary Submit an assignment (student)
// @Tags assignments
// @Security BearerAuth
// @Router /assignments/submit [post]
func (h *Handler) Submit(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req SubmitRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	sub, err := h.service.Submit(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(submissionToMap(sub))
}

// GetMySubmission godoc
// @Summary Get my submission for an assignment
// @Tags assignments
// @Security BearerAuth
// @Router /assignments/{id}/my-submission [get]
func (h *Handler) GetMySubmission(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	assignmentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	sub, err := h.service.GetMySubmission(c.Context(), userID, assignmentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not submitted"})
	}
	return c.JSON(submissionToMap(sub))
}

// ListSubmissions godoc
// @Summary List all submissions for an assignment (instructor/admin)
// @Tags assignments
// @Security BearerAuth
// @Router /assignments/{id}/submissions [get]
func (h *Handler) ListSubmissions(c fiber.Ctx) error {
	assignmentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	limit, offset := parsePagination(c)
	rows, err := h.service.ListSubmissions(c.Context(), assignmentID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":              utils.UUIDFromPg(r.ID),
			"user_id":         utils.UUIDFromPg(r.UserID),
			"email":           r.Email,
			"full_name":       utils.TextFromPg(r.FullName),
			"submission_text": utils.TextFromPg(r.SubmissionText),
			"file_path":       utils.TextFromPg(r.FilePath),
			"submitted_at":    r.SubmittedAt,
			"graded_at":       r.GradedAt,
			"marks_obtained":  utils.NumericToFloat(r.MarksObtained),
			"feedback":        utils.TextFromPg(r.Feedback),
			"status":          utils.TextFromPg(r.Status),
		}
	}
	return c.JSON(out)
}

// ListMySubmissions godoc
// @Summary List all submissions for the current user
// @Tags assignments
// @Security BearerAuth
// @Router /assignments/my-submissions [get]
func (h *Handler) ListMySubmissions(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMySubmissions(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":               utils.UUIDFromPg(r.ID),
			"assignment_id":    utils.UUIDFromPg(r.AssignmentID),
			"assignment_title": r.AssignmentTitle,
			"due_date":         r.DueDate,
			"max_marks":        utils.NumericToFloat(r.MaxMarks),
			"submission_text":  utils.TextFromPg(r.SubmissionText),
			"file_path":        utils.TextFromPg(r.FilePath),
			"submitted_at":     r.SubmittedAt,
			"graded_at":        r.GradedAt,
			"marks_obtained":   utils.NumericToFloat(r.MarksObtained),
			"feedback":         utils.TextFromPg(r.Feedback),
			"status":           utils.TextFromPg(r.Status),
		}
	}
	return c.JSON(out)
}

// Grade godoc
// @Summary Grade a submission (instructor/admin)
// @Tags assignments
// @Security BearerAuth
// @Router /assignments/submissions/{id}/grade [post]
func (h *Handler) Grade(c fiber.Ctx) error {
	graderID, _ := c.Locals("userID").(uuid.UUID)
	subID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req GradeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	sub, err := h.service.Grade(c.Context(), graderID, subID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(submissionToMap(sub))
}
