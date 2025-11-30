package stream

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

// Helper to convert pgtype.UUID to string
func uuidToString(id interface{}) string {
	switch v := id.(type) {
	case uuid.UUID:
		return v.String()
	case [16]byte:
		return uuid.UUID(v).String()
	default:
		return ""
	}
}

// CreateStream godoc
// @Summary Create a new stream
// @Description Create a new live stream (Instructor/Admin only)
// @Tags streams
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateStreamRequest true "Stream details"
// @Success 201 {object} map[string]interface{} "Stream created successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 403 {object} map[string]interface{} "Forbidden"
// @Router /streams [post]
func (h *Handler) CreateStream(c fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	var req CreateStreamRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	stream, err := h.service.CreateStream(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(stream)
}

// GetStream godoc
// @Summary Get stream details
// @Description Get details of a specific stream
// @Tags streams
// @Accept json
// @Produce json
// @Param id path string true "Stream ID"
// @Success 200 {object} map[string]interface{} "Stream details"
// @Failure 400 {object} map[string]interface{} "Invalid stream ID"
// @Failure 404 {object} map[string]interface{} "Stream not found"
// @Router /streams/{id} [get]
func (h *Handler) GetStream(c fiber.Ctx) error {
	streamID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}

	stream, err := h.service.GetStream(c.Context(), streamID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "stream not found"})
	}

	// Convert pgtype values to simple types
	id := uuid.UUID(stream.ID.Bytes).String()
	instructorID := ""
	if stream.InstructorID.Valid {
		instructorID = uuid.UUID(stream.InstructorID.Bytes).String()
	}
	description := ""
	if stream.Description.Valid {
		description = stream.Description.String
	}
	status := ""
	if stream.Status.Valid {
		status = stream.Status.String
	}
	viewerCount := int32(0)
	if stream.ViewerCount.Valid {
		viewerCount = stream.ViewerCount.Int32
	}

	// Include HLS URL for playback (always include for live streams)
	hlsURL := "http://localhost:8080/hls/" + stream.StreamKey + ".m3u8"

	return c.JSON(fiber.Map{
		"id":            id,
		"title":         stream.Title,
		"description":   description,
		"instructor_id": instructorID,
		"status":        status,
		"stream_key":    stream.StreamKey,
		"viewer_count":  viewerCount,
		"scheduled_at":  stream.ScheduledAt,
		"started_at":    stream.StartedAt,
		"ended_at":      stream.EndedAt,
		"created_at":    stream.CreatedAt,
		"hls_url":       hlsURL,
		"rtmp_url":      "rtmp://localhost:1935/live/" + stream.StreamKey,
	})
}

// ListLiveStreams godoc
// @Summary List all live streams
// @Description Get a list of all currently live streams
// @Tags streams
// @Accept json
// @Produce json
// @Success 200 {array} map[string]interface{} "List of live streams"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /streams/live [get]
func (h *Handler) ListLiveStreams(c fiber.Ctx) error {
	streams, err := h.service.ListLiveStreams(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Return empty array instead of null, and include HLS URLs
	if streams == nil || len(streams) == 0 {
		return c.JSON([]fiber.Map{})
	}

	// Add HLS URL to each stream with proper ID conversion
	result := make([]fiber.Map, len(streams))
	for i, s := range streams {
		// Convert pgtype.UUID to string
		streamID := uuid.UUID(s.ID.Bytes).String()
		instructorID := ""
		if s.InstructorID.Valid {
			instructorID = uuid.UUID(s.InstructorID.Bytes).String()
		}

		// Get description string
		description := ""
		if s.Description.Valid {
			description = s.Description.String
		}

		// Get status string
		status := ""
		if s.Status.Valid {
			status = s.Status.String
		}

		// Get viewer count
		viewerCount := int32(0)
		if s.ViewerCount.Valid {
			viewerCount = s.ViewerCount.Int32
		}

		result[i] = fiber.Map{
			"id":            streamID,
			"title":         s.Title,
			"description":   description,
			"instructor_id": instructorID,
			"status":        status,
			"stream_key":    s.StreamKey,
			"viewer_count":  viewerCount,
			"scheduled_at":  s.ScheduledAt,
			"started_at":    s.StartedAt,
			"created_at":    s.CreatedAt,
			"hls_url":       "http://localhost:8080/hls/" + s.StreamKey + ".m3u8",
		}
	}

	return c.JSON(result)
}

// StartStream godoc
// @Summary Start a stream
// @Description Start a scheduled stream (Instructor/Admin only)
// @Tags streams
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Stream ID"
// @Success 200 {object} map[string]interface{} "Stream started"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 403 {object} map[string]interface{} "Forbidden"
// @Router /streams/{id}/start [post]
func (h *Handler) StartStream(c fiber.Ctx) error {
	streamID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}

	if err := h.service.StartStream(c.Context(), streamID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "stream started"})
}

// EndStream godoc
// @Summary End a stream
// @Description End a live stream (Instructor/Admin only)
// @Tags streams
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Stream ID"
// @Success 200 {object} map[string]interface{} "Stream ended"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 403 {object} map[string]interface{} "Forbidden"
// @Router /streams/{id}/end [post]
func (h *Handler) EndStream(c fiber.Ctx) error {
	streamID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}

	if err := h.service.EndStream(c.Context(), streamID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "stream ended"})
}
