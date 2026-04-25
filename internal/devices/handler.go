package devices

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register handles POST /api/v1/devices/register.
//
//	@Summary	Register or refresh an FCM device token
//	@Tags		devices
//	@Security	BearerAuth
//	@Param		body	body	RegisterInput	true	"Token + platform"
//	@Success	200		{object}	map[string]interface{}
//	@Router		/devices/register [post]
func (h *Handler) Register(c fiber.Ctx) error {
	var in RegisterInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	userID, _ := c.Locals("userID").(uuid.UUID)
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	if err := h.svc.Register(c.Context(), tenantID, userID, in); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"registered": true})
}

// Unregister handles DELETE /api/v1/devices/:token. Used on logout.
//
//	@Summary	Drop a device token
//	@Tags		devices
//	@Security	BearerAuth
//	@Router		/devices/{token} [delete]
func (h *Handler) Unregister(c fiber.Ctx) error {
	token := c.Params("token")
	if err := h.svc.Unregister(c.Context(), token); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"unregistered": true})
}
