package assignments

import (
	"strconv"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func ts(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

func listRow(id, courseID, batchID pgtype.UUID, title string, desc, attach pgtype.Text, due pgtype.Timestamptz, marks pgtype.Numeric, status db.PublishStatus) fiber.Map {
	return fiber.Map{
		"id":             utils.UUIDFromPg(id),
		"course_id":      utils.UUIDFromPg(courseID),
		"batch_id":       utils.UUIDFromPg(batchID),
		"title":          title,
		"description":    utils.TextFromPg(desc),
		"attachment_url": utils.TextFromPg(attach),
		"due_at":         ts(due),
		"due_date":       ts(due),
		"max_marks":      utils.NumericToFloat(marks),
		"status":         string(status),
		"is_published":   status == db.PublishStatusPublished,
	}
}

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

// Create — POST /assignments  (instructor/admin)
func (h *Handler) Create(c fiber.Ctx) error {
	var req CreateAssignmentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	a, err := h.service.Create(c.Context(), h.tenant(c), h.user(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": utils.UUIDFromPg(a.ID), "title": a.Title, "status": string(a.Status),
		"due_at": ts(a.DueAt), "max_marks": utils.NumericToFloat(a.MaxMarks),
	})
}

// Get — GET /assignments/:id
func (h *Handler) Get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	a, err := h.service.Get(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{
		"id":             utils.UUIDFromPg(a.ID),
		"course_id":      utils.UUIDFromPg(a.CourseID),
		"batch_id":       utils.UUIDFromPg(a.BatchID),
		"lesson_id":      utils.UUIDFromPg(a.LessonID),
		"title":          a.Title,
		"description":    utils.TextFromPg(a.Description),
		"attachment_url": utils.TextFromPg(a.AttachmentUrl),
		"due_at":         ts(a.DueAt),
		"due_date":       ts(a.DueAt),
		"max_marks":      utils.NumericToFloat(a.MaxMarks),
		"status":         string(a.Status),
		"is_published":   a.Status == db.PublishStatusPublished,
	})
}

func (h *Handler) list(c fiber.Ctx, courseID, batchID, createdBy *uuid.UUID) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.List(c.Context(), h.tenant(c), courseID, batchID, createdBy, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = listRow(r.ID, r.CourseID, r.BatchID, r.Title, r.Description, r.AttachmentUrl, r.DueAt, r.MaxMarks, r.Status)
	}
	return c.JSON(out)
}

// ListByBatch — GET /assignments/batch/:batch_id
func (h *Handler) ListByBatch(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("batch_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid batch id"})
	}
	return h.list(c, nil, &id, nil)
}

// ListByCourse — GET /assignments/course/:course_id
func (h *Handler) ListByCourse(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	return h.list(c, &id, nil, nil)
}

// ListMine — GET /assignments/mine  (instructor: assignments I created)
func (h *Handler) ListMine(c fiber.Ctx) error {
	uid := h.user(c)
	return h.list(c, nil, nil, &uid)
}

// Update — PUT /assignments/:id
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
	return c.JSON(listRow(a.ID, pgtype.UUID{}, pgtype.UUID{}, a.Title, a.Description, a.AttachmentUrl, a.DueAt, a.MaxMarks, a.Status))
}

// Delete — DELETE /assignments/:id  (soft)
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

// Submit — POST /assignments/submit  (student)
func (h *Handler) Submit(c fiber.Ctx) error {
	var req SubmitRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.AssignmentID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "assignment_id required"})
	}
	s, err := h.service.Submit(c.Context(), h.tenant(c), h.user(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": utils.UUIDFromPg(s.ID), "assignment_id": utils.UUIDFromPg(s.AssignmentID),
		"status": s.Status, "submitted_at": ts(s.SubmittedAt),
	})
}

// GetMySubmission — GET /assignments/:id/my-submission
func (h *Handler) GetMySubmission(c fiber.Ctx) error {
	assignmentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	s, err := h.service.GetMySubmission(c.Context(), h.user(c), assignmentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not submitted"})
	}
	return c.JSON(fiber.Map{
		"id":              utils.UUIDFromPg(s.ID),
		"submission_text": utils.TextFromPg(s.SubmissionText),
		"file_key":        utils.TextFromPg(s.FileKey),
		"submitted_at":    ts(s.SubmittedAt),
		"marks_obtained":  utils.NumericToFloat(s.MarksObtained),
		"feedback":        utils.TextFromPg(s.Feedback),
		"status":          s.Status,
	})
}

// ListSubmissions — GET /assignments/:id/submissions  (instructor/admin)
func (h *Handler) ListSubmissions(c fiber.Ctx) error {
	assignmentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	rows, err := h.service.ListSubmissions(c.Context(), assignmentID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":             utils.UUIDFromPg(r.ID),
			"user_id":        utils.UUIDFromPg(r.UserID),
			"full_name":      utils.TextFromPg(r.FullName),
			"phone":          utils.TextFromPg(r.Phone),
			"submitted_at":   ts(r.SubmittedAt),
			"marks_obtained": utils.NumericToFloat(r.MarksObtained),
			"status":         r.Status,
		}
	}
	return c.JSON(out)
}

// ListMySubmissions — GET /assignments/my-submissions  (student)
func (h *Handler) ListMySubmissions(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMySubmissions(c.Context(), h.tenant(c), h.user(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":               utils.UUIDFromPg(r.ID),
			"assignment_id":    utils.UUIDFromPg(r.AssignmentID),
			"assignment_title": r.AssignmentTitle,
			"max_marks":        utils.NumericToFloat(r.MaxMarks),
			"submission_text":  utils.TextFromPg(r.SubmissionText),
			"file_key":         utils.TextFromPg(r.FileKey),
			"submitted_at":     ts(r.SubmittedAt),
			"marks_obtained":   utils.NumericToFloat(r.MarksObtained),
			"feedback":         utils.TextFromPg(r.Feedback),
			"status":           r.Status,
		}
	}
	return c.JSON(out)
}

// Grade — POST /assignments/submissions/:id/grade  (instructor/admin)
func (h *Handler) Grade(c fiber.Ctx) error {
	subID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req GradeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	s, err := h.service.Grade(c.Context(), h.user(c), subID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"id": utils.UUIDFromPg(s.ID), "marks_obtained": utils.NumericToFloat(s.MarksObtained),
		"status": s.Status, "graded_at": ts(s.GradedAt),
	})
}
