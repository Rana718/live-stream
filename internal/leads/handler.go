package leads

import (
	"live-platform/internal/database"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// POST /public/leads
func (h *Handler) Create(c fiber.Ctx) error {
	var in CreateLeadInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if in.Name == "" || in.Phone == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name + phone required"})
	}
	row, err := h.svc.Create(c.Context(), in)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "thanks — our team will reach out soon", "lead": row,
	})
}

// POST /public/leads/:id/booking-intent
func (h *Handler) BookingIntent(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		Slot string `json:"slot"`
	}
	_ = c.Bind().Body(&body)
	if body.Slot == "" {
		body.Slot = "unspecified"
	}
	if err := h.svc.UpdateStatus(database.WithSuperAdmin(c.Context()), id, "contacted", uuid.Nil,
		"Booking intent: "+body.Slot+" slot picked from website"); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// GET /admin/leads  (super_admin)
func (h *Handler) List(c fiber.Ctx) error {
	rows, err := h.svc.List(c.Context(), c.Query("status"), 50, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// PATCH /admin/leads/:id  (super_admin)
func (h *Handler) UpdateLead(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Status     string `json:"status"`
		AssignedTo string `json:"assigned_to"`
		Notes      string `json:"notes"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	assignee := uuid.Nil
	if req.AssignedTo != "" {
		assignee, _ = uuid.Parse(req.AssignedTo)
	}
	if err := h.svc.UpdateStatus(c.Context(), id, req.Status, assignee, req.Notes); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}
