package recording

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetRecording(c fiber.Ctx) error {
	recordingID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid recording id"})
	}

	recording, err := h.service.GetRecording(c.Context(), recordingID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "recording not found"})
	}

	return c.JSON(recording)
}

func (h *Handler) GetRecordingsByStream(c fiber.Ctx) error {
	streamID, err := uuid.Parse(c.Params("stream_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}

	recordings, err := h.service.GetRecordingsByStream(c.Context(), streamID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(recordings)
}

func (h *Handler) GetRecordingURL(c fiber.Ctx) error {
	recordingID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid recording id"})
	}

	url, err := h.service.GetRecordingURL(c.Context(), recordingID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "recording not found"})
	}

	return c.JSON(fiber.Map{"url": url})
}
