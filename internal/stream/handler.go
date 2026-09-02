package stream

import (
	"live-platform/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// CreateStream — POST /streams  (instructor/admin)
func (h *Handler) CreateStream(c fiber.Ctx) error {
	var req CreateStreamRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	sess, err := h.service.CreateStream(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(sess)
}

// GetStream — GET /streams/:id
func (h *Handler) GetStream(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}
	sess, err := h.service.Get(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "stream not found"})
	}
	return c.JSON(sess)
}

// ListLiveStreams — GET /streams/live
func (h *Handler) ListLiveStreams(c fiber.Ctx) error {
	tid := middleware.CurrentTenantID(c)
	if tid == uuid.Nil {
		if v := c.Query("tenant_id"); v != "" {
			if p, e := uuid.Parse(v); e == nil {
				tid = p
			}
		}
	}
	sessions, err := h.service.ListLive(c.Context(), tid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(sessions)
}

// StartStream — POST /streams/:id/start
func (h *Handler) StartStream(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}
	if err := h.service.Start(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "stream started"})
}

// EndStream — POST /streams/:id/end
func (h *Handler) EndStream(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}
	if err := h.service.End(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "stream ended"})
}
