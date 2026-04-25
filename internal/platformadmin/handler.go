package platformadmin

import (
	"strconv"
	"time"

	"live-platform/internal/payments"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	svc       *Service
	jwtSecret string
}

// NewHandler — jwtSecret is required for impersonation. Pass cfg.JWT.AccessSecret.
func NewHandler(svc *Service, jwtSecret string) *Handler {
	return &Handler{svc: svc, jwtSecret: jwtSecret}
}

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

// GetFeatures — GET /api/v1/admin/platform/tenants/:id/features
func (h *Handler) GetFeatures(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	raw, err := h.svc.GetFeatures(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	c.Set("Content-Type", "application/json")
	return c.Send(raw)
}

// SetFeatures — PUT /api/v1/admin/platform/tenants/:id/features
//
// Body is the full feature-flag JSON object. Replace-all semantics — the
// caller GETs, mutates, then PUTs back.
func (h *Handler) SetFeatures(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	body := c.Body()
	if len(body) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "empty body"})
	}
	out, err := h.svc.SetFeatures(c.Context(), id, body)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	c.Set("Content-Type", "application/json")
	return c.Send(out)
}

// CreateRazorpayAccount — POST /api/v1/admin/platform/tenants/:id/razorpay/create
//
// Body: payments.CreateLinkedAccountInput. On success the tenant's
// `razorpay_account_id` is updated automatically — no follow-up PUT needed.
func (h *Handler) CreateRazorpayAccount(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var in payments.CreateLinkedAccountInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	acc, err := h.svc.CreateLinkedAccount(c.Context(), id, in)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(acc)
}

// SetCustomDomain — PUT /api/v1/admin/platform/tenants/:id/domain
//
// Body: { "domain": "learn.rajan.com" }  (empty string to detach)
func (h *Handler) SetCustomDomain(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		Domain string `json:"domain"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	t, err := h.svc.SetCustomDomain(c.Context(), id, body.Domain)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(t)
}

// SetRazorpayAccount — PUT /api/v1/admin/platform/tenants/:id/razorpay
//
// Body: { "account_id": "acc_XXXXXXXXXXXX" }  (empty string to detach)
func (h *Handler) SetRazorpayAccount(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		AccountID string `json:"account_id"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	t, err := h.svc.SetRazorpayAccount(c.Context(), id, body.AccountID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(t)
}

// Impersonate — POST /api/v1/admin/platform/tenants/:id/impersonate
//
// Mints a 15-minute access token for the tenant's owner so support can
// drop into the tenant's admin portal without their password. Audit logs
// pick this up automatically as a mutating super_admin action.
//
//	@Summary  Super-admin: impersonate a tenant_admin for support
//	@Tags     platformadmin
//	@Security BearerAuth
//	@Router   /admin/platform/tenants/{id}/impersonate [post]
func (h *Handler) Impersonate(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	res, err := h.svc.Impersonate(c.Context(), id, h.jwtSecret)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
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
