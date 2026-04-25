package whatsapp

import (
	"github.com/gofiber/fiber/v3"
)

// Handler exposes the broadcast endpoint. Mounted under SuperAdminContext
// in main.go — only platform staff can fire wholesale broadcasts.
type Handler struct {
	client Client
}

func NewHandler(client Client) *Handler { return &Handler{client: client} }

type broadcastRequest struct {
	Recipients []string `json:"recipients" validate:"required,min=1"`
	Message    string   `json:"message" validate:"required,min=1,max=4096"`
}

// Broadcast — POST /api/v1/admin/platform/whatsapp/broadcast
//
//	@Summary  Super-admin: WhatsApp broadcast to a recipient list
//	@Tags     whatsapp
//	@Security BearerAuth
//	@Param    body body broadcastRequest true "phones + message"
//	@Success  200 {object} map[string]interface{}
//	@Router   /admin/platform/whatsapp/broadcast [post]
func (h *Handler) Broadcast(c fiber.Ctx) error {
	if h.client == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "whatsapp provider not configured",
			"hint":  "set WHATSAPP_PROVIDER=gupshup + WHATSAPP_API_KEY",
		})
	}
	var req broadcastRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if len(req.Recipients) == 0 || req.Message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "recipients + message required"})
	}
	sent, err := h.client.Broadcast(c.Context(), req.Recipients, req.Message)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
			"sent":  sent,
		})
	}
	return c.JSON(fiber.Map{
		"sent":      sent,
		"requested": len(req.Recipients),
	})
}
