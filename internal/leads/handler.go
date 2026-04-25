package leads

import "github.com/gofiber/fiber/v3"

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
