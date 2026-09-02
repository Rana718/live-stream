package bookmarks

import (
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

// Create — POST /bookmarks
func (h *Handler) Create(c fiber.Ctx) error {
	var req CreateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	row, err := h.service.Create(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	lid := utils.UUIDFromPg(row.LessonID)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":                utils.UUIDFromPg(row.ID),
		"lesson_id":         lid,
		"lecture_id":        lid,
		"position_sec":      row.PositionSec,
		"timestamp_seconds": row.PositionSec,
		"note":              utils.TextFromPg(row.Note),
		"created_at":        row.CreatedAt.Time,
	})
}

// ListMine — GET /bookmarks
func (h *Handler) ListMine(c fiber.Ctx) error {
	rows, err := h.service.ListMine(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		lid := utils.UUIDFromPg(r.LessonID)
		out[i] = fiber.Map{
			"id":                utils.UUIDFromPg(r.ID),
			"lesson_id":         lid,
			"lecture_id":        lid,
			"position_sec":      r.PositionSec,
			"timestamp_seconds": r.PositionSec,
			"note":              utils.TextFromPg(r.Note),
			"created_at":        r.CreatedAt.Time,
		}
	}
	return c.JSON(out)
}

// ListForLecture — GET /bookmarks/lecture/:lecture_id. schema-v2 no longer
// filters bookmarks per lesson server-side; return the full list and let the
// client narrow. (Retired in Phase J.)
func (h *Handler) ListForLecture(c fiber.Ctx) error {
	return h.ListMine(c)
}

// Delete — DELETE /bookmarks/:id
func (h *Handler) Delete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Delete(c.Context(), id, middleware.CurrentUserID(c)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}
