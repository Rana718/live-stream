package appbuilds

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit, err := strconv.Atoi(c.Query("limit", "50"))
	if err != nil || limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}
	return int32(limit), int32(offset)
}

// Trigger — POST /api/v1/admin/platform/tenants/:id/build
//
//	@Summary  Super-admin: kick off a white-label app build
//	@Tags     appbuilds
//	@Security BearerAuth
//	@Router   /admin/platform/tenants/{id}/build [post]
func (h *Handler) Trigger(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var in TriggerInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	row, err := h.svc.Trigger(c.Context(), id, in)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusAccepted).JSON(row)
}

// List — GET /api/v1/admin/platform/builds?status=queued
func (h *Handler) List(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.svc.List(c.Context(), c.Query("status"), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// PatchStatus — PATCH /api/v1/admin/platform/builds/:id  (super_admin)
// Also reachable from the Codemagic webhook (signature-verified).
func (h *Handler) PatchStatus(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var in SetStatusInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	row, err := h.svc.SetStatus(c.Context(), id, in)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(row)
}
