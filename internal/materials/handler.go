package materials

import (
	"strconv"
	"time"

	"live-platform/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func (h *Handler) tenant(c fiber.Ctx) uuid.UUID { return middleware.CurrentTenantID(c) }

// Upload — POST /materials/upload  (instructor/admin)
func (h *Handler) Upload(c fiber.Ctx) error {
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
	}
	if req.Title == "" {
		req.Title = file.Filename
	}
	ct := file.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	m, err := h.service.Upload(c.Context(), h.tenant(c), middleware.CurrentUserID(c), req, file.Filename, file.Size, f, ct)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(m)
}

// Get — GET /materials/:id
func (h *Handler) Get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	m, err := h.service.Get(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(m)
}

// GetDownloadURL — GET /materials/:id/download
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

// ListByChapter / ListByTopic — GET /materials/chapter/:id, /materials/topic/:id.
// schema-v2 no longer scopes documents by chapter/topic; return the tenant's
// document library. (Retired in Phase J.)
func (h *Handler) ListByChapter(c fiber.Ctx) error { return h.listAll(c) }
func (h *Handler) ListByTopic(c fiber.Ctx) error   { return h.listAll(c) }

func (h *Handler) listAll(c fiber.Ctx) error {
	limit := int32(100)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
		limit = int32(l)
	}
	rows, err := h.service.ListForTenant(c.Context(), h.tenant(c), limit, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// Delete — DELETE /materials/:id
func (h *Handler) Delete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Delete(c.Context(), h.tenant(c), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}
