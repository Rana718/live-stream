package notifications

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

func tv(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit, offset := int32(30), int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

func (h *Handler) tenant(c fiber.Ctx) uuid.UUID { return middleware.CurrentTenantID(c) }
func (h *Handler) user(c fiber.Ctx) uuid.UUID   { return middleware.CurrentUserID(c) }

// ListMine — GET /notifications
func (h *Handler) ListMine(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMine(c.Context(), h.tenant(c), h.user(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, n := range rows {
		out[i] = fiber.Map{
			"id":            utils.UUIDFromPg(n.ID),
			"type":          n.TemplateKey,
			"template_key":  n.TemplateKey,
			"title":         n.Title,
			"body":          utils.TextFromPg(n.Body),
			"entity_type":   utils.TextFromPg(n.EntityType),
			"resource_type": utils.TextFromPg(n.EntityType),
			"entity_id":     utils.UUIDFromPg(n.EntityID),
			"resource_id":   utils.UUIDFromPg(n.EntityID),
			"is_read":       n.ReadAt.Valid,
			"read_at":       tv(n.ReadAt),
			"created_at":    tv(n.CreatedAt),
		}
	}
	return c.JSON(out)
}

// UnreadCount — GET /notifications/unread-count
func (h *Handler) UnreadCount(c fiber.Ctx) error {
	n, err := h.service.UnreadCount(c.Context(), h.tenant(c), h.user(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"unread": n})
}

// MarkRead — POST /notifications/:id/read
func (h *Handler) MarkRead(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.MarkRead(c.Context(), id, h.user(c)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "ok"})
}

// MarkAllRead — POST /notifications/read-all
func (h *Handler) MarkAllRead(c fiber.Ctx) error {
	if err := h.service.MarkAllRead(c.Context(), h.tenant(c), h.user(c)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "ok"})
}

// Delete — DELETE /notifications/:id
func (h *Handler) Delete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Delete(c.Context(), id, h.user(c)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// AdminSend — POST /admin/notifications/send
func (h *Handler) AdminSend(c fiber.Ctx) error {
	var req struct {
		UserID       uuid.UUID  `json:"user_id"`
		Type         string     `json:"type"`
		Title        string     `json:"title"`
		Body         string     `json:"body"`
		ResourceType string     `json:"resource_type"`
		ResourceID   *uuid.UUID `json:"resource_id"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.UserID == uuid.Nil || req.Title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id and title required"})
	}
	if req.Type == "" {
		req.Type = "admin"
	}
	n, err := h.service.Create(c.Context(), h.tenant(c), req.UserID, req.Type, req.Title, req.Body, req.ResourceType, req.ResourceID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": utils.UUIDFromPg(n.ID)})
}

// CreateAnnouncement — POST /announcements
func (h *Handler) CreateAnnouncement(c fiber.Ctx) error {
	var req CreateAnnouncementRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	a, err := h.service.CreateAnnouncement(c.Context(), h.tenant(c), h.user(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": utils.UUIDFromPg(a.ID), "title": a.Title, "body": a.Body, "priority": a.Priority,
	})
}

func (h *Handler) listAnnouncements(c fiber.Ctx, courseID, batchID *uuid.UUID) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListAnnouncements(c.Context(), h.tenant(c), courseID, batchID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, a := range rows {
		out[i] = fiber.Map{
			"id":           utils.UUIDFromPg(a.ID),
			"course_id":    utils.UUIDFromPg(a.CourseID),
			"batch_id":     utils.UUIDFromPg(a.BatchID),
			"title":        a.Title,
			"body":         a.Body,
			"priority":     a.Priority,
			"published_at": tv(a.PublishedAt),
			"expires_at":   tv(a.ExpiresAt),
		}
	}
	return c.JSON(out)
}

// ListGlobal — GET /announcements
func (h *Handler) ListGlobal(c fiber.Ctx) error { return h.listAnnouncements(c, nil, nil) }

// ListBatch — GET /announcements/batch/:batch_id
func (h *Handler) ListBatch(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("batch_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid batch id"})
	}
	return h.listAnnouncements(c, nil, &id)
}

// ListCourse — GET /announcements/course/:course_id
func (h *Handler) ListCourse(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	return h.listAnnouncements(c, &id, nil)
}

// DeleteAnnouncement — DELETE /announcements/:id
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
