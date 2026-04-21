package materials

import (
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func toMap(m *db.StudyMaterial) fiber.Map {
	return fiber.Map{
		"id":             utils.UUIDFromPg(m.ID),
		"topic_id":       utils.UUIDFromPg(m.TopicID),
		"chapter_id":     utils.UUIDFromPg(m.ChapterID),
		"subject_id":     utils.UUIDFromPg(m.SubjectID),
		"title":          m.Title,
		"description":    utils.TextFromPg(m.Description),
		"material_type":  utils.TextFromPg(m.MaterialType),
		"file_path":      m.FilePath,
		"file_size":      utils.Int8FromPg(m.FileSize),
		"language":       utils.TextFromPg(m.Language),
		"is_free":        utils.BoolFromPg(m.IsFree),
		"download_count": utils.Int4FromPg(m.DownloadCount),
		"created_at":     m.CreatedAt,
	}
}

// Upload godoc
// @Summary Upload a study material file (instructor/admin)
// @Tags materials
// @Accept multipart/form-data
// @Security BearerAuth
// @Param file formData file true "File"
// @Param title formData string true "Title"
// @Param chapter_id formData string false "Chapter ID"
// @Param topic_id formData string false "Topic ID"
// @Param material_type formData string false "pdf|doc|ppt|image"
// @Param is_free formData bool false "Is free"
// @Router /materials/upload [post]
func (h *Handler) Upload(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file required"})
	}
	f, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "cannot open file"})
	}
	defer f.Close()

	req := UploadRequest{
		Title:        c.FormValue("title"),
		Description:  c.FormValue("description"),
		MaterialType: c.FormValue("material_type"),
		Language:     c.FormValue("language"),
		IsFree:       c.FormValue("is_free") == "true",
	}
	if v := c.FormValue("chapter_id"); v != "" {
		id, err := uuid.Parse(v)
		if err == nil {
			req.ChapterID = &id
		}
	}
	if v := c.FormValue("topic_id"); v != "" {
		id, err := uuid.Parse(v)
		if err == nil {
			req.TopicID = &id
		}
	}
	if v := c.FormValue("subject_id"); v != "" {
		id, err := uuid.Parse(v)
		if err == nil {
			req.SubjectID = &id
		}
	}

	ct := file.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	m, err := h.service.Upload(c.Context(), userID, req, file.Filename, file.Size, f, ct)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(toMap(m))
}

func (h *Handler) Get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	m, err := h.service.Get(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(toMap(m))
}

func (h *Handler) GetDownloadURL(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	url, err := h.service.GetDownloadURL(c.Context(), id, 15*time.Minute)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"url": url, "expires_in": 900})
}

func (h *Handler) ListByChapter(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("chapter_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid chapter id"})
	}
	rows, err := h.service.ListByChapter(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = toMap(&rows[i])
	}
	return c.JSON(out)
}

func (h *Handler) ListByTopic(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("topic_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid topic id"})
	}
	rows, err := h.service.ListByTopic(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = toMap(&rows[i])
	}
	return c.JSON(out)
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
