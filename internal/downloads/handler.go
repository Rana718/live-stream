package downloads

import (
	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func variantToMap(v *db.VideoVariant) fiber.Map {
	return fiber.Map{
		"id":           utils.UUIDFromPg(v.ID),
		"recording_id": utils.UUIDFromPg(v.RecordingID),
		"lecture_id":   utils.UUIDFromPg(v.LectureID),
		"quality":      v.Quality,
		"file_path":    v.FilePath,
		"file_size":    utils.Int8FromPg(v.FileSize),
		"bitrate_kbps": utils.Int4FromPg(v.BitrateKbps),
		"width":        utils.Int4FromPg(v.Width),
		"height":       utils.Int4FromPg(v.Height),
		"codec":        utils.TextFromPg(v.Codec),
	}
}

// CreateVariant godoc
// @Summary Register a transcoded video variant (admin/instructor)
// @Tags downloads
// @Security BearerAuth
// @Router /downloads/variants [post]
func (h *Handler) CreateVariant(c fiber.Ctx) error {
	var req CreateVariantRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	v, err := h.service.CreateVariant(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(variantToMap(v))
}

// ListVariantsForLecture godoc
// @Summary List all video qualities available for a lecture
// @Tags downloads
// @Router /downloads/lectures/{lecture_id}/variants [get]
func (h *Handler) ListVariantsForLecture(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("lecture_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid lecture id"})
	}
	rows, err := h.service.ListVariantsForLecture(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = variantToMap(&rows[i])
	}
	return c.JSON(out)
}

// IssueToken godoc
// @Summary Get a time-limited download token for offline use
// @Tags downloads
// @Security BearerAuth
// @Router /downloads/token [post]
func (h *Handler) IssueToken(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req TokenRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	resp, err := h.service.IssueToken(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

// Fetch godoc
// @Summary Redirect to a presigned URL using a download token
// @Tags downloads
// @Router /downloads/fetch [get]
func (h *Handler) Fetch(c fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token required"})
	}
	url, err := h.service.Resolve(c.Context(), token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Redirect().To(url)
}
