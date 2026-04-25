package tenants

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// LookupByOrgCode handles GET /api/v1/public/tenants/by-code/:code
// Public — no JWT required. Mounted with PublicLookupContext middleware.
//
//	@Summary	Resolve an org code to public tenant info (logo + theme)
//	@Tags		tenants
//	@Param		code	path	string	true	"Org code"
//	@Success	200		{object}	PublicTenantInfo
//	@Router		/public/tenants/by-code/{code} [get]
func (h *Handler) LookupByOrgCode(c fiber.Ctx) error {
	code := c.Params("code")
	info, err := h.svc.LookupByOrgCode(c.Context(), code)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(info)
}

// MyTenant handles GET /api/v1/tenants/me
// Returns the tenant record for the authenticated user. Used by web/mobile
// to fetch full theming once logged in (LookupByOrgCode is for pre-login).
//
//	@Summary	Current tenant for the authenticated user
//	@Tags		tenants
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]interface{}
//	@Router		/tenants/me [get]
func (h *Handler) MyTenant(c fiber.Ctx) error {
	tenantID, ok := c.Locals("tenantID").(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no tenant"})
	}
	t, err := h.svc.MyTenant(c.Context(), tenantID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(t)
}

// Features handles GET /api/v1/tenants/me/features
//
//	@Summary	Feature flags for the current tenant
//	@Tags		tenants
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]interface{}
//	@Router		/tenants/me/features [get]
func (h *Handler) Features(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	feats, err := h.svc.GetFeatures(c.Context(), tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	c.Set("Content-Type", "application/json")
	return c.Send(feats)
}

// UpdateBranding handles PUT /api/v1/tenants/me/branding (tenant_admin role).
//
//	@Summary	Update tenant logo + theme
//	@Tags		tenants
//	@Security	BearerAuth
//	@Param		body	body	UpdateBrandingRequest	true	"Branding"
//	@Success	200		{object}	map[string]interface{}
//	@Router		/tenants/me/branding [put]
func (h *Handler) UpdateBranding(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	var req UpdateBrandingRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	t, err := h.svc.UpdateBranding(c.Context(), tenantID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(t)
}

// CreateTenant handles POST /api/v1/admin/tenants  (super_admin only).
//
//	@Summary	Provision a new tenant
//	@Tags		tenants
//	@Security	BearerAuth
//	@Param		body	body	CreateTenantRequest	true	"Tenant"
//	@Success	201		{object}	map[string]interface{}
//	@Router		/admin/tenants [post]
func (h *Handler) CreateTenant(c fiber.Ctx) error {
	var req CreateTenantRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	ownerID, _ := c.Locals("userID").(uuid.UUID)
	t, err := h.svc.Create(c.Context(), req, ownerID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(t)
}
