package schedule

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Create — POST /api/v1/admin/class-schedules
//
//	@Summary  Create a recurring class schedule
//	@Tags     class_schedules
//	@Security BearerAuth
func (h *Handler) Create(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	var in CreateInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	row, err := h.svc.Create(c.Context(), tenantID, in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(row)
}

// List — GET /api/v1/admin/class-schedules?active=true|false
func (h *Handler) List(c fiber.Ctx) error {
	var active *bool
	if v := c.Query("active"); v != "" {
		b := v == "true" || v == "1"
		active = &b
	}
	rows, err := h.svc.List(c.Context(), active, 100, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// SetActive — PATCH /api/v1/admin/class-schedules/:id/active
func (h *Handler) SetActive(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		Active bool `json:"active"`
	}
	_ = c.Bind().Body(&body)
	if err := h.svc.SetActive(c.Context(), id, body.Active); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"updated": true})
}

// Delete — DELETE /api/v1/admin/class-schedules/:id
func (h *Handler) Delete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.Delete(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"deleted": true})
}
