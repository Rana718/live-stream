package recording

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// UploadRecording godoc
// @Summary Upload a recording
// @Description Upload a recorded stream video (Instructor/Admin only)
// @Tags recordings
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param stream_id formData string true "Stream ID"
// @Param file formData file true "Recording file"
// @Success 201 {object} map[string]interface{} "Recording uploaded successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Upload failed"
// @Router /recordings/upload [post]
func (h *Handler) UploadRecording(c fiber.Ctx) error {
	streamID, err := uuid.Parse(c.FormValue("stream_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file required"})
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to open file"})
	}
	defer src.Close()

	// Generate unique filename
	filename := fmt.Sprintf("recordings/%s/%s.webm", streamID.String(), uuid.New().String())

	// Upload to MinIO
	recording, err := h.service.UploadRecording(c.Context(), streamID, filename, src, file.Size)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":   "recording uploaded successfully",
		"recording": recording,
	})
}

// GetRecording godoc
// @Summary Get recording details
// @Description Get details of a specific recording
// @Tags recordings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Recording ID"
// @Success 200 {object} map[string]interface{} "Recording details"
// @Failure 400 {object} map[string]interface{} "Invalid recording ID"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 404 {object} map[string]interface{} "Recording not found"
// @Router /recordings/{id} [get]
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

// GetRecordingsByStream godoc
// @Summary Get recordings by stream
// @Description Get all recordings for a specific stream
// @Tags recordings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param stream_id path string true "Stream ID"
// @Success 200 {array} map[string]interface{} "List of recordings"
// @Failure 400 {object} map[string]interface{} "Invalid stream ID"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /recordings/stream/{stream_id} [get]
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

// GetRecordingURL godoc
// @Summary Get recording playback URL
// @Description Get a presigned URL for recording playback
// @Tags recordings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Recording ID"
// @Success 200 {object} map[string]interface{} "Playback URL"
// @Failure 400 {object} map[string]interface{} "Invalid recording ID"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 404 {object} map[string]interface{} "Recording not found"
// @Router /recordings/{id}/url [get]
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

// GetMyRecordings godoc
// @Summary Get instructor's recordings
// @Description Get all recordings for the logged-in instructor
// @Tags recordings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {array} map[string]interface{} "List of recordings"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /recordings/my [get]
func (h *Handler) GetMyRecordings(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	recordings, err := h.service.GetRecordingsByInstructor(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if recordings == nil {
		recordings = []map[string]interface{}{}
	}

	return c.JSON(recordings)
}
