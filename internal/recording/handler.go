package recording

import (
	"fmt"
	"strconv"

	"live-platform/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) tenant(c fiber.Ctx) uuid.UUID { return middleware.CurrentTenantID(c) }

func pagination(c fiber.Ctx) (int32, int32) {
	limit := int32(50)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 200 {
		limit = int32(l)
	}
	return limit, 0
}

// UploadRecording — POST /recordings/upload  (instructor/admin)
func (h *Handler) UploadRecording(c fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.FormValue("session_id"))
	if err != nil {
		if sessionID, err = uuid.Parse(c.FormValue("stream_id")); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session id"})
		}
	}
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file required"})
	}
	src, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to open file"})
	}
	defer src.Close()
	key := fmt.Sprintf("recordings/%s/%s.webm", sessionID.String(), uuid.New().String())
	rec, err := h.service.UploadRecording(c.Context(), h.tenant(c), sessionID, key, src, file.Size)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "recording uploaded successfully", "recording": rec})
}

// GetRecording — GET /recordings/:id
func (h *Handler) GetRecording(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid recording id"})
	}
	rec, err := h.service.Get(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "recording not found"})
	}
	return c.JSON(rec)
}

// GetRecordingsByStream — GET /recordings/stream/:stream_id
func (h *Handler) GetRecordingsByStream(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("stream_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session id"})
	}
	rows, err := h.service.BySession(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// GetRecordingURL — GET /recordings/:id/url
func (h *Handler) GetRecordingURL(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid recording id"})
	}
	url, err := h.service.GetURL(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "recording not found"})
	}
	return c.JSON(fiber.Map{"url": url})
}

// GetMyRecordings — GET /recordings/my  (instructor: recordings of my sessions)
func (h *Handler) GetMyRecordings(c fiber.Ctx) error {
	limit, offset := pagination(c)
	rows, err := h.service.ForInstructor(c.Context(), h.tenant(c), middleware.CurrentUserID(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// ListMine — GET /recordings/mine  (student: recordings for my enrolled courses)
func (h *Handler) ListMine(c fiber.Ctx) error {
	limit, offset := pagination(c)
	rows, err := h.service.ForUser(c.Context(), h.tenant(c), middleware.CurrentUserID(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}
