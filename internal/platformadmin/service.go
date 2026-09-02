// Package platformadmin is the super_admin (platform-staff) control plane.
// Every method runs under SuperAdminContext middleware (is_super_admin() =
// true), so tenant RLS is bypassed and reads span every tenant.
//
// Conceptually mirrors what a tenant admin can do for one org, except scoped
// across every tenant: triage marketing leads, suspend abusive tenants, read
// platform-wide stats, impersonate a tenant admin for support.
package platformadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/payments"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RazorpayLinker is the slice of the Razorpay client we depend on. Tiny
// interface so tests can pass a fake and we sidestep an import cycle.
type RazorpayLinker interface {
	CreateLinkedAccount(ctx context.Context, in payments.CreateLinkedAccountInput) (*payments.LinkedAccount, error)
}

type Service struct {
	q  *db.Queries
	rp RazorpayLinker
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

// WithRazorpay enables the Razorpay-Account auto-create flow. Optional —
// the manual paste-an-account-ID path keeps working without it.
func (s *Service) WithRazorpay(rp RazorpayLinker) *Service { s.rp = rp; return s }

// ─────────────────────────────────────────────────────────────── tenants

// ListTenants returns every tenant with its member count. Optional `status`
// filter mirrors the `tenants.status` enum (trial|active|past_due|suspended|
// cancelled).
func (s *Service) ListTenants(ctx context.Context, status string, limit, offset int32) ([]db.PlatformListTenantsRow, error) {
	var st db.NullTenantStatus
	if status != "" {
		st = db.NullTenantStatus{TenantStatus: db.TenantStatus(status), Valid: true}
	}
	return s.q.PlatformListTenants(ctx, db.PlatformListTenantsParams{
		Status: st,
		Limit:  limit,
		Offset: offset,
	})
}

// SuspendTenant flips status to 'suspended'. Auth issuance must check this
// before minting tokens for the tenant's users.
func (s *Service) SuspendTenant(ctx context.Context, id uuid.UUID) error {
	return s.q.SetTenantStatus(ctx, db.SetTenantStatusParams{
		ID: utils.UUIDToPg(id), Status: db.TenantStatusSuspended,
	})
}

func (s *Service) ReactivateTenant(ctx context.Context, id uuid.UUID) error {
	return s.q.SetTenantStatus(ctx, db.SetTenantStatusParams{
		ID: utils.UUIDToPg(id), Status: db.TenantStatusActive,
	})
}

// UpdateTenantPlan moves a tenant between plans and (usually) flips it live
// after a platform-subscription payment lands.
func (s *Service) UpdateTenantPlan(ctx context.Context, id uuid.UUID, plan, status string, trialEnds *time.Time) (*db.PlatformUpdateTenantPlanRow, error) {
	if status == "" {
		status = "active"
	}
	trial := pgtype.Timestamptz{}
	if trialEnds != nil {
		trial = pgtype.Timestamptz{Time: *trialEnds, Valid: true}
	}
	row, err := s.q.PlatformUpdateTenantPlan(ctx, db.PlatformUpdateTenantPlanParams{
		ID:          utils.UUIDToPg(id),
		Plan:        db.TenantPlan(plan),
		Status:      db.TenantStatus(status),
		TrialEndsAt: trial,
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ─────────────────────────────────────────────────────────── stats / audit

func (s *Service) PlatformStats(ctx context.Context) (*db.PlatformTenantStatsRow, error) {
	row, err := s.q.PlatformTenantStats(ctx)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) LeadStats(ctx context.Context) (*db.PlatformLeadStatsRow, error) {
	row, err := s.q.PlatformLeadStats(ctx)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) RecentSignups(ctx context.Context, limit int32) ([]db.PlatformRecentSignupsRow, error) {
	return s.q.PlatformRecentSignups(ctx, limit)
}

func (s *Service) PlatformAuditLogs(ctx context.Context, limit, offset int32) ([]db.ListAuditLogsRow, error) {
	return s.q.ListAuditLogs(ctx, db.ListAuditLogsParams{
		Limit: limit, Offset: offset, TenantID: pgtype.UUID{}, // null → every tenant
	})
}

// ─────────────────────────────────────────────────────────── platform users

// UserFilter narrows the cross-tenant user list.
type UserFilter struct {
	TenantID *uuid.UUID
	Role     string
	Query    string
}

func (s *Service) ListUsers(ctx context.Context, f UserFilter, limit, offset int32) ([]db.PlatformListUsersRow, int64, error) {
	tid := pgtype.UUID{}
	if f.TenantID != nil {
		tid = utils.UUIDToPg(*f.TenantID)
	}
	var role db.NullTenantRole
	if f.Role != "" {
		role = db.NullTenantRole{TenantRole: db.TenantRole(f.Role), Valid: true}
	}
	q := pgtype.Text{}
	if strings.TrimSpace(f.Query) != "" {
		q = pgtype.Text{String: strings.TrimSpace(f.Query), Valid: true}
	}
	rows, err := s.q.PlatformListUsers(ctx, db.PlatformListUsersParams{
		Limit: limit, Offset: offset, TenantID: tid, Role: role, Q: q,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.PlatformCountUsers(ctx, db.PlatformCountUsersParams{
		TenantID: tid, Role: role, Q: q,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ─────────────────────────────────────────────────────── features / domain

// GetFeatures returns a tenant's feature-flag JSON. Empty `{}` if no row.
func (s *Service) GetFeatures(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	row, err := s.q.GetTenantSettings(ctx, utils.UUIDToPg(tenantID))
	if err != nil || len(row.Features) == 0 {
		return []byte("{}"), nil
	}
	return row.Features, nil
}

// SetFeatures replaces the feature-flag JSON for a tenant.
func (s *Service) SetFeatures(ctx context.Context, tenantID uuid.UUID, features []byte) ([]byte, error) {
	if len(features) == 0 {
		features = []byte("{}")
	}
	row, err := s.q.UpsertTenantSettings(ctx, db.UpsertTenantSettingsParams{
		TenantID: utils.UUIDToPg(tenantID),
		Features:  features,
	})
	if err != nil {
		return nil, err
	}
	return row.Features, nil
}

// SetCustomDomain attaches (or, with an empty string, detaches) a tenant's
// primary custom domain. Super-admin action — the domain is marked verified
// immediately.
func (s *Service) SetCustomDomain(ctx context.Context, id uuid.UUID, domain string) error {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return fmt.Errorf("domain required (detach by deleting the domain row directly)")
	}
	return s.q.SetTenantPrimaryDomain(ctx, db.SetTenantPrimaryDomainParams{
		TenantID: utils.UUIDToPg(id),
		Domain:   domain,
	})
}

// SetRazorpayAccount stores a tenant's Route Linked-Account ID so future
// purchases auto-split. Empty string detaches.
func (s *Service) SetRazorpayAccount(ctx context.Context, id uuid.UUID, accountID string) (*db.SetTenantRazorpayAccountRow, error) {
	row, err := s.q.SetTenantRazorpayAccount(ctx, db.SetTenantRazorpayAccountParams{
		ID:                utils.UUIDToPg(id),
		RazorpayAccountID: accountID,
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// CreateLinkedAccount provisions a Razorpay Route account for a tenant and
// stores the resulting ID.
func (s *Service) CreateLinkedAccount(ctx context.Context, tenantID uuid.UUID, in payments.CreateLinkedAccountInput) (*payments.LinkedAccount, error) {
	if s.rp == nil {
		return nil, fmt.Errorf("razorpay client not wired")
	}
	if in.ReferenceID == "" {
		in.ReferenceID = tenantID.String()
	}
	acc, err := s.rp.CreateLinkedAccount(ctx, in)
	if err != nil {
		return nil, err
	}
	if _, err := s.SetRazorpayAccount(ctx, tenantID, acc.ID); err != nil {
		return nil, err
	}
	return acc, nil
}

// ─────────────────────────────────────────────────────────── build config

// BuildConfig is the branding bundle Codemagic fetches before each per-tenant
// build. Hand-picked fields — never the raw tenant row.
type BuildConfig struct {
	TenantID    string          `json:"tenant_id"`
	OrgCode     string          `json:"org_code"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	PackageID   string          `json:"package_id"`
	VersionName string          `json:"version_name"`
	Theme       json.RawMessage `json:"theme"`
	LogoURL     string          `json:"logo_url,omitempty"`
}

func (s *Service) GetBuildConfig(ctx context.Context, tenantID uuid.UUID, platform string) (*BuildConfig, error) {
	t, err := s.q.GetTenantByID(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return nil, fmt.Errorf("tenant not found")
	}
	cfg := &BuildConfig{
		TenantID:    utils.UUIDFromPg(t.ID),
		OrgCode:     t.OrgCode,
		Name:        t.Name,
		Slug:        t.Slug,
		PackageID:   "com.school." + strings.ReplaceAll(t.Slug, "-", ""),
		VersionName: time.Now().Format("2006.01.02"),
		Theme:       t.Theme,
	}
	if t.LogoUrl.Valid {
		cfg.LogoURL = t.LogoUrl.String
	}
	return cfg, nil
}

// ─────────────────────────────────────────────────────────── impersonation

type ImpersonationResult struct {
	AccessToken string    `json:"access_token"`
	TenantID    uuid.UUID `json:"tenant_id"`
	TenantName  string    `json:"tenant_name"`
	OrgCode     string    `json:"org_code"`
	UserID      uuid.UUID `json:"user_id"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Impersonate mints a 15-minute admin access token for the tenant's owner
// (or first admin) so platform support can drop into their portal.
func (s *Service) Impersonate(ctx context.Context, tenantID uuid.UUID, jwtSecret string) (*ImpersonationResult, error) {
	t, err := s.q.GetTenantByID(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return nil, err
	}

	var targetID uuid.UUID
	if t.OwnerUserID.Valid {
		targetID = uuid.UUID(t.OwnerUserID.Bytes)
	}
	if targetID == uuid.Nil {
		adminRole := db.NullTenantRole{TenantRole: db.TenantRoleAdmin, Valid: true}
		members, e := s.q.ListTenantMembers(ctx, db.ListTenantMembersParams{
			TenantID: utils.UUIDToPg(tenantID),
			Role:     adminRole,
			Limit:    1,
			Offset:   0,
		})
		if e != nil || len(members) == 0 {
			return nil, fmt.Errorf("no admin user in tenant %s", tenantID)
		}
		targetID = uuid.UUID(members[0].UserID.Bytes)
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	tok, err := utils.GenerateAccessToken(targetID, t.Name+"@impersonated", "admin",
		tenantID, 0, jwtSecret, time.Until(expiresAt))
	if err != nil {
		return nil, err
	}
	return &ImpersonationResult{
		AccessToken: tok,
		TenantID:    tenantID,
		TenantName:  t.Name,
		OrgCode:     t.OrgCode,
		UserID:      targetID,
		ExpiresAt:   expiresAt,
	}, nil
}
