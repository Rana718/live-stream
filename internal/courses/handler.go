package courses

import (
	"strconv"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func courseToMap(c *db.Course) fiber.Map {
	return fiber.Map{
		"id":               utils.UUIDFromPg(c.ID),
		"exam_category_id": utils.UUIDFromPg(c.ExamCategoryID),
		"title":            c.Title,
		"slug":             c.Slug,
		"description":      utils.TextFromPg(c.Description),
		"thumbnail_url":    utils.TextFromPg(c.ThumbnailUrl),
		"price":            utils.NumericToFloat(c.Price),
		"discounted_price": utils.NumericToFloat(c.DiscountedPrice),
		"duration_months":  utils.Int4FromPg(c.DurationMonths),
		"language":         utils.TextFromPg(c.Language),
		"level":            utils.TextFromPg(c.Level),
		"is_free":          utils.BoolFromPg(c.IsFree),
		"is_published":     utils.BoolFromPg(c.IsPublished),
		"created_by":       utils.UUIDFromPg(c.CreatedBy),
		"created_at":       c.CreatedAt,
		"updated_at":       c.UpdatedAt,
	}
}

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

// ListCourses godoc
// @Summary List published courses
// @Tags courses
// @Param limit query int false "Page size"
// @Param offset query int false "Offset"
// @Param exam_category query string false "Exam category UUID"
// @Param language query string false "Language code"
// @Param q query string false "Search query"
// @Success 200 {array} map[string]interface{}
// @Router /courses [get]
func (h *Handler) List(c fiber.Ctx) error {
	limit, offset := parsePagination(c)

	if q := c.Query("q"); q != "" {
		out, err := h.service.Search(c.Context(), q, limit, offset)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return renderCourseList(c, out)
	}
	if ec := c.Query("exam_category"); ec != "" {
		ecID, err := uuid.Parse(ec)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid exam_category"})
		}
		out, err := h.service.ListByExamCategory(c.Context(), ecID, limit, offset)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return renderCourseList(c, out)
	}
	if lang := c.Query("language"); lang != "" {
		out, err := h.service.ListByLanguage(c.Context(), lang, limit, offset)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return renderCourseList(c, out)
	}
	out, err := h.service.ListPublished(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return renderCourseList(c, out)
}

func renderCourseList(c fiber.Ctx, rows []db.Course) error {
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = courseToMap(&rows[i])
	}
	return c.JSON(out)
}

// GetCourse godoc
// @Summary Get a course by id or slug
// @Tags courses
// @Router /courses/{id} [get]
func (h *Handler) Get(c fiber.Ctx) error {
	idParam := c.Params("id")
	if id, err := uuid.Parse(idParam); err == nil {
		course, err := h.service.Get(c.Context(), id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
		}
		return c.JSON(courseToMap(course))
	}
	course, err := h.service.GetBySlug(c.Context(), idParam)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(courseToMap(course))
}

// CreateCourse godoc
// @Summary Create a course (instructor/admin)
// @Tags courses
// @Security BearerAuth
// @Router /courses [post]
func (h *Handler) Create(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req CreateCourseRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	course, err := h.service.Create(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(courseToMap(course))
}

// UpdateCourse godoc
// @Summary Update a course (instructor/admin)
// @Tags courses
// @Security BearerAuth
// @Router /courses/{id} [put]
func (h *Handler) Update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req CreateCourseRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	course, err := h.service.Update(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(courseToMap(course))
}

// DeleteCourse godoc
// @Summary Delete a course (admin)
// @Tags courses
// @Security BearerAuth
// @Router /courses/{id} [delete]
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
