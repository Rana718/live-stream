package lectures

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

func toMap(l *db.Lecture) fiber.Map {
	return fiber.Map{
		"id":               utils.UUIDFromPg(l.ID),
		"topic_id":         utils.UUIDFromPg(l.TopicID),
		"chapter_id":       utils.UUIDFromPg(l.ChapterID),
		"subject_id":       utils.UUIDFromPg(l.SubjectID),
		"title":            l.Title,
		"description":      utils.TextFromPg(l.Description),
		"lecture_type":     utils.TextFromPg(l.LectureType),
		"instructor_id":    utils.UUIDFromPg(l.InstructorID),
		"stream_id":        utils.UUIDFromPg(l.StreamID),
		"recording_id":     utils.UUIDFromPg(l.RecordingID),
		"thumbnail_url":    utils.TextFromPg(l.ThumbnailUrl),
		"scheduled_at":     l.ScheduledAt,
		"duration_seconds": utils.Int4FromPg(l.DurationSeconds),
		"language":         utils.TextFromPg(l.Language),
		"is_free":          utils.BoolFromPg(l.IsFree),
		"is_published":     utils.BoolFromPg(l.IsPublished),
		"display_order":    utils.Int4FromPg(l.DisplayOrder),
		"view_count":       utils.Int4FromPg(l.ViewCount),
		"created_at":       l.CreatedAt,
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

func (h *Handler) Create(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req CreateLectureRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.InstructorID == nil {
		req.InstructorID = &userID
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	l, err := h.service.Create(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(toMap(l))
}

func (h *Handler) List(c fiber.Ctx) error {
	limit, offset := parsePagination(c)

	if q := c.Query("q"); q != "" {
		rows, err := h.service.Search(c.Context(), q, limit, offset)
		return renderOrError(c, rows, err)
	}
	if t := c.Query("topic_id"); t != "" {
		id, err := uuid.Parse(t)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid topic_id"})
		}
		rows, err := h.service.ListByTopic(c.Context(), id)
		return renderOrError(c, rows, err)
	}
	if ch := c.Query("chapter_id"); ch != "" {
		id, err := uuid.Parse(ch)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid chapter_id"})
		}
		rows, err := h.service.ListByChapter(c.Context(), id)
		return renderOrError(c, rows, err)
	}
	if sj := c.Query("subject_id"); sj != "" {
		id, err := uuid.Parse(sj)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid subject_id"})
		}
		rows, err := h.service.ListBySubject(c.Context(), id)
		return renderOrError(c, rows, err)
	}
	if c.Query("live") == "true" {
		rows, err := h.service.ListLive(c.Context(), limit, offset)
		return renderOrError(c, rows, err)
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "provide one of: topic_id, chapter_id, subject_id, live=true, or q"})
}

func renderOrError(c fiber.Ctx, rows []db.Lecture, err error) error {
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = toMap(&rows[i])
	}
	return c.JSON(out)
}

func (h *Handler) Get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	l, err := h.service.Get(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	_ = h.service.IncrementView(c.Context(), id)
	return c.JSON(toMap(l))
}

func (h *Handler) Update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req CreateLectureRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	l, err := h.service.Update(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(toMap(l))
}

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

// RecordWatch godoc
// @Summary Record a lecture watch event (for progress tracking)
// @Security BearerAuth
// @Router /lectures/watch [post]
func (h *Handler) RecordWatch(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req RecordWatchRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.service.RecordWatch(c.Context(), userID, req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "ok"})
}

// History godoc
// @Summary Get my viewing history
// @Security BearerAuth
// @Router /lectures/history [get]
func (h *Handler) History(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	limit, offset := parsePagination(c)
	rows, err := h.service.ListHistory(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"lecture_id":       utils.UUIDFromPg(r.LectureID),
			"title":            r.Title,
			"thumbnail_url":    utils.TextFromPg(r.ThumbnailUrl),
			"duration_seconds": utils.Int4FromPg(r.DurationSeconds),
			"watched_seconds":  utils.Int4FromPg(r.WatchedSeconds),
			"completed":        utils.BoolFromPg(r.Completed),
			"last_watched_at":  r.LastWatchedAt,
		}
	}
	return c.JSON(out)
}
