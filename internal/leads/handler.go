package leads

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Create handles POST /api/v1/public/leads.
//
//	@Summary	Public lead capture (marketing site)
//	@Tags		leads
//	@Param		body	body	CreateLeadInput	true	"Prospect form fields"
//	@Success	201		{object}	map[string]interface{}
//	@Router		/public/leads [post]
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
		"message": "thanks — our team will reach out soon",
		"lead":    row,
	})
}

// BookingIntent handles POST /api/v1/public/leads/:id/booking-intent
//
// Called from the marketing demo page after the user picks a Cal.com slot
// type ("quick" or "deep"). Bumps the lead status to 'demo' so sales can
// triage. We don't validate the slot enum strictly — the only consumer is
// our own marketing site and the worst case is a janky note string.
//
//	@Summary	Record marketing demo slot intent
//	@Tags		leads
//	@Param		id	path	string	true	"Lead UUID"
//	@Router		/public/leads/{id}/booking-intent [post]
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
	lead, err := h.svc.MarkBookingIntent(c.Context(),
		pgtype.UUID{Bytes: id, Valid: true}, body.Slot)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"lead": lead})
}

// List handles GET /api/v1/admin/leads (super_admin only).
//
//	@Summary	Triage marketing leads
//	@Tags		leads
//	@Security	BearerAuth
//	@Router		/admin/leads [get]
func (h *Handler) List(c fiber.Ctx) error {
	status := c.Query("status")
	limit := int32(50)
	offset := int32(0)
	rows, err := h.svc.List(c.Context(), status, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}
