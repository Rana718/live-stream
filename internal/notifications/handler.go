package notifications

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

func notifToMap(n *db.Notification) fiber.Map {
	return fiber.Map{
		"id":            utils.UUIDFromPg(n.ID),
		"type":          n.Type,
		"title":         n.Title,
		"body":          utils.TextFromPg(n.Body),
		"resource_type": utils.TextFromPg(n.ResourceType),
		"resource_id":   utils.UUIDFromPg(n.ResourceID),
		"is_read":       utils.BoolFromPg(n.IsRead),
		"created_at":    n.CreatedAt,
		"read_at":       n.ReadAt,
	}
}

func announcementToMap(a *db.Announcement) fiber.Map {
	return fiber.Map{
		"id":           utils.UUIDFromPg(a.ID),
		"batch_id":     utils.UUIDFromPg(a.BatchID),
		"course_id":    utils.UUIDFromPg(a.CourseID),
		"created_by":   utils.UUIDFromPg(a.CreatedBy),
		"title":        a.Title,
		"body":         a.Body,
		"priority":     utils.TextFromPg(a.Priority),
		"published_at": a.PublishedAt,
		"expires_at":   a.ExpiresAt,
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

// ListMine godoc
// @Summary List my notifications
// @Tags notifications
// @Security BearerAuth
// @Router /notifications [get]
func (h *Handler) ListMine(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMine(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = notifToMap(&rows[i])
	}
	return c.JSON(out)
}

// UnreadCount godoc
// @Summary Count of my unread notifications
// @Tags notifications
// @Security BearerAuth
// @Router /notifications/unread-count [get]
func (h *Handler) UnreadCount(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	n, err := h.service.UnreadCount(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"unread": n})
}

// MarkRead godoc
// @Summary Mark a notification read
// @Tags notifications
// @Security BearerAuth
// @Router /notifications/{id}/read [post]
func (h *Handler) MarkRead(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.MarkRead(c.Context(), id, userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "ok"})
}

// MarkAllRead godoc
// @Summary Mark all my notifications read
// @Tags notifications
// @Security BearerAuth
// @Router /notifications/read-all [post]
func (h *Handler) MarkAllRead(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	if err := h.service.MarkAllRead(c.Context(), userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "ok"})
}

// Delete godoc
// @Summary Delete a notification
// @Tags notifications
// @Security BearerAuth
// @Router /notifications/{id} [delete]
func (h *Handler) Delete(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Delete(c.Context(), id, userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// AdminSend godoc
// @Summary Admin: push a notification to a specific user
// @Tags notifications
// @Security BearerAuth
// @Router /admin/notifications/send [post]
func (h *Handler) AdminSend(c fiber.Ctx) error {
	var req struct {
		UserID       uuid.UUID  `json:"user_id" validate:"required"`
		Type         string     `json:"type" validate:"required"`
		Title        string     `json:"title" validate:"required"`
		Body         string     `json:"body"`
		ResourceType string     `json:"resource_type"`
		ResourceID   *uuid.UUID `json:"resource_id"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	n, err := h.service.Create(c.Context(), req.UserID, req.Type, req.Title, req.Body, req.ResourceType, req.ResourceID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(notifToMap(n))
}

// CreateAnnouncement godoc
// @Summary Create a (possibly fan-out) announcement (instructor/admin)
// @Tags announcements
// @Security BearerAuth
// @Router /announcements [post]
func (h *Handler) CreateAnnouncement(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req CreateAnnouncementRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	a, err := h.service.CreateAnnouncement(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(announcementToMap(a))
}

// ListGlobal godoc
// @Summary List global announcements
// @Tags announcements
// @Router /announcements [get]
func (h *Handler) ListGlobal(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListGlobalAnnouncements(c.Context(), limit, offset)
	return renderAnnouncements(c, rows, err)
}

// ListBatch godoc
// @Summary List announcements for a batch
// @Tags announcements
// @Router /announcements/batch/{batch_id} [get]
func (h *Handler) ListBatch(c fiber.Ctx) error {
	batchID, err := uuid.Parse(c.Params("batch_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid batch id"})
	}
	limit, offset := parsePagination(c)
	rows, err := h.service.ListBatchAnnouncements(c.Context(), batchID, limit, offset)
	return renderAnnouncements(c, rows, err)
}

// ListCourse godoc
// @Summary List announcements for a course
// @Tags announcements
// @Router /announcements/course/{course_id} [get]
func (h *Handler) ListCourse(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	limit, offset := parsePagination(c)
	rows, err := h.service.ListCourseAnnouncements(c.Context(), courseID, limit, offset)
	return renderAnnouncements(c, rows, err)
}

func renderAnnouncements(c fiber.Ctx, rows []db.Announcement, err error) error {
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = announcementToMap(&rows[i])
	}
	return c.JSON(out)
}

// DeleteAnnouncement godoc
// @Summary Delete an announcement (admin)
// @Tags announcements
// @Security BearerAuth
// @Router /announcements/{id} [delete]
func (h *Handler) DeleteAnnouncement(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.DeleteAnnouncement(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}
