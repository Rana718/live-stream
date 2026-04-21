package bookmarks

import (
	"strconv"

	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit := int32(20)
	offset := int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// Create godoc
// @Summary Add a bookmark (for a lecture timestamp or a material page)
// @Tags bookmarks
// @Security BearerAuth
// @Router /bookmarks [post]
func (h *Handler) Create(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req CreateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	b, err := h.service.Create(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":                utils.UUIDFromPg(b.ID),
		"lecture_id":        utils.UUIDFromPg(b.LectureID),
		"material_id":       utils.UUIDFromPg(b.MaterialID),
		"timestamp_seconds": utils.Int4FromPg(b.TimestampSeconds),
		"note":              utils.TextFromPg(b.Note),
		"created_at":        b.CreatedAt,
	})
}

// ListMine godoc
// @Summary List my bookmarks
// @Tags bookmarks
// @Security BearerAuth
// @Router /bookmarks [get]
func (h *Handler) ListMine(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMine(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":                utils.UUIDFromPg(r.ID),
			"lecture_id":        utils.UUIDFromPg(r.LectureID),
			"lecture_title":     utils.TextFromPg(r.LectureTitle),
			"material_id":       utils.UUIDFromPg(r.MaterialID),
			"material_title":    utils.TextFromPg(r.MaterialTitle),
			"timestamp_seconds": utils.Int4FromPg(r.TimestampSeconds),
			"note":              utils.TextFromPg(r.Note),
			"created_at":        r.CreatedAt,
		}
	}
	return c.JSON(out)
}

// ListForLecture godoc
// @Summary Get my bookmarks for a single lecture
// @Tags bookmarks
// @Security BearerAuth
// @Router /bookmarks/lecture/{lecture_id} [get]
func (h *Handler) ListForLecture(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("lecture_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid lecture id"})
	}
	rows, err := h.service.ListForLecture(c.Context(), userID, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":                utils.UUIDFromPg(r.ID),
			"timestamp_seconds": utils.Int4FromPg(r.TimestampSeconds),
			"note":              utils.TextFromPg(r.Note),
			"created_at":        r.CreatedAt,
		}
	}
	return c.JSON(out)
}

// Delete godoc
// @Summary Delete a bookmark
// @Tags bookmarks
// @Security BearerAuth
// @Router /bookmarks/{id} [delete]
func (h *Handler) Delete(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Delete(c.Context(), id, userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}
