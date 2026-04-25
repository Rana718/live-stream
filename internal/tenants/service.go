// Package tenants implements the multi-tenant control plane: tenant
// onboarding, branding, plan management, feature flags, and the public
// Org Code → tenant lookup that the marketing/login surfaces hit before any
// JWT is issued.
package tenants

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{queries: db.New(pool), pool: pool}
}

// PublicTenantInfo is the tenant payload returned by the public Org Code
// lookup. We deliberately omit anything that would help an attacker enumerate
// or impersonate the tenant (owner_user_id, razorpay_account_id, internal
// status flags).
type PublicTenantInfo struct {
	ID       uuid.UUID       `json:"id"`
	OrgCode  string          `json:"org_code"`
	Name     string          `json:"name"`
	Slug     string          `json:"slug"`
	LogoURL  string          `json:"logo_url,omitempty"`
	Theme    json.RawMessage `json:"theme"`
	Plan     string          `json:"plan"`
}

// LookupByOrgCode is called by an unauthenticated client (login screen,
// marketing site) to translate an Org Code to branding data. Uses the
// public-lookup RLS policy so the conn doesn't need a tenant_id session var.
func (s *Service) LookupByOrgCode(ctx context.Context, code string) (*PublicTenantInfo, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, fmt.Errorf("org code required")
	}

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT set_config('app.is_public_lookup', 'true', false)"); err != nil {
		return nil, err
	}

	q := db.New(conn)
	t, err := q.GetTenantByOrgCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("tenant not found")
	}

	info := &PublicTenantInfo{
		ID:      uuid.UUID(t.ID.Bytes),
		OrgCode: t.OrgCode,
		Name:    t.Name,
		Slug:    t.Slug,
		Plan:    t.Plan,
		Theme:   t.Theme,
	}
	if t.LogoUrl.Valid {
		info.LogoURL = t.LogoUrl.String
	}
	return info, nil
}

// CreateTenantRequest is the payload that comes from the marketing
// onboarding form. orgCode and slug must be unique platform-wide.
type CreateTenantRequest struct {
	OrgCode    string `json:"org_code" validate:"required,min=4,max=20"`
	Name       string `json:"name" validate:"required,max=200"`
	Slug       string `json:"slug" validate:"required,min=3,max=100"`
	OwnerEmail string `json:"owner_email" validate:"required,email"`
	Plan       string `json:"plan"`
}

// Create provisions a new tenant. Caller must have super_admin role; this
// runs under the SuperAdminContext so the tenants insert bypasses RLS.
func (s *Service) Create(ctx context.Context, req CreateTenantRequest, ownerUserID uuid.UUID) (*db.Tenant, error) {
	defaultTheme := []byte(`{
		"primary": "#6C4AD0",
		"primaryDark": "#5A3BB5",
		"accent": "#FFE0EA",
		"background": "#F7F7FB"
	}`)
	defaultConfig := []byte(`{}`)

	t, err := s.queries.CreateTenant(ctx, db.CreateTenantParams{
		OrgCode:     strings.ToUpper(req.OrgCode),
		Name:        req.Name,
		Slug:        strings.ToLower(req.Slug),
		Column4:     req.Plan, // plan (NULLIF in query falls back to 'starter')
		OwnerUserID: pgtype.UUID{Bytes: ownerUserID, Valid: ownerUserID != uuid.Nil},
		Theme:       defaultTheme,
		AppConfig:   defaultConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}

	// Seed default feature flags.
	defaultFeatures := []byte(`{
		"live": true,
		"store": true,
		"tests": true,
		"ai_doubts": false,
		"downloads": false
	}`)
	_, _ = s.queries.UpsertTenantFeatures(ctx, db.UpsertTenantFeaturesParams{
		TenantID: t.ID,
		Features: defaultFeatures,
	})

	return &t, nil
}

// UpdateBrandingRequest carries the per-tenant theming the admin dashboard
// edits. Only the theme JSON validates; logo_url is presumed already uploaded
// to MinIO by a separate /uploads endpoint.
type UpdateBrandingRequest struct {
	Name    string          `json:"name"`
	LogoURL string          `json:"logo_url"`
	Theme   json.RawMessage `json:"theme"`
}

func (s *Service) UpdateBranding(ctx context.Context, tenantID uuid.UUID, req UpdateBrandingRequest) (*db.Tenant, error) {
	theme := req.Theme
	if len(theme) == 0 {
		theme = []byte(`{}`)
	}
	t, err := s.queries.UpdateTenantBranding(ctx, db.UpdateTenantBrandingParams{
		ID:      pgtype.UUID{Bytes: tenantID, Valid: true},
		Column2: req.Name,
		Column3: req.LogoURL,
		Theme:   theme,
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// MyTenant returns the tenant record for the authenticated user. Used by
// the admin dashboard to render the current org banner and to check feature
// flags.
func (s *Service) MyTenant(ctx context.Context, tenantID uuid.UUID) (*db.Tenant, error) {
	t, err := s.queries.GetTenantByID(ctx, pgtype.UUID{Bytes: tenantID, Valid: true})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetFeatures returns the JSON feature-flag bag for a tenant. Empty if no row.
func (s *Service) GetFeatures(ctx context.Context, tenantID uuid.UUID) (json.RawMessage, error) {
	raw, err := s.queries.GetTenantFeatures(ctx, pgtype.UUID{Bytes: tenantID, Valid: true})
	if err != nil {
		return []byte(`{}`), nil
	}
	return raw, nil
}
