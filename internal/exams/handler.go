package exams

import (
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

// ListExamCategories godoc
// @Summary List all exam categories
// @Tags exams
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Router /exam-categories [get]
func (h *Handler) List(c fiber.Ctx) error {
	cats, err := h.service.List(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(cats))
	for i, cat := range cats {
		out[i] = fiber.Map{
			"id":            utils.UUIDFromPg(cat.ID),
			"name":          cat.Name,
			"slug":          cat.Slug,
			"description":   utils.TextFromPg(cat.Description),
			"icon_url":      utils.TextFromPg(cat.IconUrl),
			"display_order": utils.Int4FromPg(cat.DisplayOrder),
			"is_active":     utils.BoolFromPg(cat.IsActive),
		}
	}
	return c.JSON(out)
}

// CreateExamCategory godoc
// @Summary Create an exam category (admin)
// @Tags exams
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body UpsertCategoryRequest true "Category"
// @Success 201 {object} map[string]interface{}
// @Router /exam-categories [post]
func (h *Handler) Create(c fiber.Ctx) error {
	var req UpsertCategoryRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	cat, err := h.service.Create(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(cat)
}

// UpdateExamCategory godoc
// @Summary Update exam category (admin)
// @Tags exams
// @Security BearerAuth
// @Router /exam-categories/{id} [put]
func (h *Handler) Update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req UpsertCategoryRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	cat, err := h.service.Update(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(cat)
}

// DeleteExamCategory godoc
// @Summary Delete exam category (admin)
// @Tags exams
// @Security BearerAuth
// @Router /exam-categories/{id} [delete]
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
