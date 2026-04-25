package platformadmin

import (
	"strconv"
	"time"

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

// ListTenants — GET /api/v1/admin/platform/tenants?status=active
//
//	@Summary  Super-admin: list every tenant on the platform
//	@Tags     platformadmin
//	@Security BearerAuth
//	@Router   /admin/platform/tenants [get]
func (h *Handler) ListTenants(c fiber.Ctx) error {
	status := c.Query("status")
	limit, offset := parsePagination(c)
	rows, err := h.svc.ListTenants(c.Context(), status, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// Suspend — POST /api/v1/admin/platform/tenants/:id/suspend
func (h *Handler) Suspend(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.SuspendTenant(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"suspended": true})
}

// Reactivate — POST /api/v1/admin/platform/tenants/:id/reactivate
func (h *Handler) Reactivate(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.ReactivateTenant(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"reactivated": true})
}

// UpdatePlan — PUT /api/v1/admin/platform/tenants/:id/plan
func (h *Handler) UpdatePlan(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		Plan        string     `json:"plan"`
		Status      string     `json:"status"`
		TrialEndsAt *time.Time `json:"trial_ends_at"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.Status == "" {
		body.Status = "active"
	}
	t, err := h.svc.UpdateTenantPlan(c.Context(), id, body.Plan, body.Status, body.TrialEndsAt)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(t)
}

// Stats — GET /api/v1/admin/platform/stats
func (h *Handler) Stats(c fiber.Ctx) error {
	tenants, err := h.svc.PlatformStats(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	leads, _ := h.svc.LeadStats(c.Context())
	signups, _ := h.svc.RecentSignups(c.Context(), 10)
	return c.JSON(fiber.Map{
		"tenants":         tenants,
		"leads":           leads,
		"recent_signups":  signups,
	})
}

// AuditLogs — GET /api/v1/admin/platform/audit
func (h *Handler) AuditLogs(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.svc.PlatformAuditLogs(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// UpdateLeadStatus — PATCH /api/v1/admin/leads/:id
func (h *Handler) UpdateLeadStatus(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		Status     string  `json:"status"`
		Notes      string  `json:"notes"`
		AssignedTo *string `json:"assigned_to"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	var assigned *uuid.UUID
	if body.AssignedTo != nil && *body.AssignedTo != "" {
		if u, e := uuid.Parse(*body.AssignedTo); e == nil {
			assigned = &u
		}
	}
	row, err := h.svc.UpdateLeadStatus(c.Context(), id, body.Status, body.Notes, assigned)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(row)
}
