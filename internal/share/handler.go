package share

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// CoursePoster — GET /api/v1/share/courses/:id/poster.png
//
// Public endpoint (no auth). Returns a 1200×630 PNG suitable for Open
// Graph, Twitter Card, WhatsApp link previews. CDN should cache by URL.
//
//	@Summary  Server-rendered course share card
//	@Tags     share
//	@Produce  png
//	@Param    id path string true "Course UUID"
//	@Success  200 {file} png
//	@Router   /share/courses/{id}/poster.png [get]
func (h *Handler) CoursePoster(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	png, err := h.svc.Render(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	c.Set("Content-Type", "image/png")
	c.Set("Cache-Control", "public, max-age=3600, s-maxage=86400")
	return c.Send(png)
}
