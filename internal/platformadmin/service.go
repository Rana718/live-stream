// Package platformadmin is the super_admin (platform-staff) control plane.
// Every method bypasses tenant RLS — callers must run inside the
// SuperAdminContext middleware.
//
// Conceptually mirrors what the tenant admin can do for one org, except
// scoped across every tenant on the platform. Used by the marketing dashboard
// to triage leads, the support tooling to suspend abusive tenants, and the
// finance dashboard to read MRR/active-tenant counts.
package platformadmin

import (
	"context"
	"fmt"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

// ListTenants returns the platform-wide tenant table joined with their
// active platform subscription (if any) and member count. Optional `status`
// filter mirrors the field on `tenants` (active|trial|suspended).
func (s *Service) ListTenants(ctx context.Context, status string, limit, offset int32) ([]db.PlatformListTenantsRow, error) {
	return s.q.PlatformListTenants(ctx, db.PlatformListTenantsParams{
		Column1: status,
		Limit:   limit,
		Offset:  offset,
	})
}

// SuspendTenant flips status to 'suspended'. The tenant's RLS still works
// from their existing session vars but every authenticated request from a
// suspended tenant should be 403'd by an upstream gate that calls IsSuspended
// before issuing tokens.
func (s *Service) SuspendTenant(ctx context.Context, id uuid.UUID) error {
	return s.q.SuspendTenant(ctx, utils.UUIDToPg(id))
}

func (s *Service) ReactivateTenant(ctx context.Context, id uuid.UUID) error {
	return s.q.ReactivateTenant(ctx, utils.UUIDToPg(id))
}

// UpdateTenantPlan moves a tenant between Starter/Pro/Premium/Enterprise.
// Used after a successful platform-subscription payment lands.
func (s *Service) UpdateTenantPlan(ctx context.Context, id uuid.UUID, plan, status string, trialEnds *time.Time) (*db.Tenant, error) {
	trial := pgtype.Timestamptz{}
	if trialEnds != nil {
		trial = pgtype.Timestamptz{Time: *trialEnds, Valid: true}
	}
	t, err := s.q.UpdateTenantPlan(ctx, db.UpdateTenantPlanParams{
		ID:          utils.UUIDToPg(id),
		Plan:        plan,
		Status:      status,
		TrialEndsAt: trial,
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// PlatformStats is the headline number bag shown on the super-admin
// dashboard (active tenants, total users, MRR, etc.).
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

func (s *Service) PlatformAuditLogs(ctx context.Context, limit, offset int32) ([]db.PlatformAuditLogsRow, error) {
	return s.q.PlatformAuditLogs(ctx, db.PlatformAuditLogsParams{Limit: limit, Offset: offset})
}

// UpsertPlatformSubInput is a super-admin record of how we charge a tenant.
type UpsertPlatformSubInput struct {
	TenantID               uuid.UUID  `json:"tenant_id"`
	Plan                   string     `json:"plan"`
	Status                 string     `json:"status"`
	AmountPaise            int        `json:"amount_paise"`
	CurrentPeriodEnd       *time.Time `json:"current_period_end"`
	TrialEndsAt            *time.Time `json:"trial_ends_at"`
	RazorpaySubscriptionID string     `json:"razorpay_subscription_id"`
}

// UpsertPlatformSubscription records (or updates) the platform's billing
// of one tenant. Mirrors the resulting plan onto the tenant row so the
// rest of the system can feature-gate without a join.
func (s *Service) UpsertPlatformSubscription(ctx context.Context, in UpsertPlatformSubInput) (*db.PlatformSubscription, error) {
	cpe := pgtype.Timestamptz{}
	if in.CurrentPeriodEnd != nil {
		cpe = pgtype.Timestamptz{Time: *in.CurrentPeriodEnd, Valid: true}
	}
	tea := pgtype.Timestamptz{}
	if in.TrialEndsAt != nil {
		tea = pgtype.Timestamptz{Time: *in.TrialEndsAt, Valid: true}
	}
	row, err := s.q.UpsertPlatformSubscription(ctx, db.UpsertPlatformSubscriptionParams{
		TenantID:               utils.UUIDToPg(in.TenantID),
		Plan:                   in.Plan,
		Status:                 in.Status,
		CurrentPeriodEnd:       cpe,
		RazorpaySubscriptionID: pgtype.Text{String: in.RazorpaySubscriptionID, Valid: in.RazorpaySubscriptionID != ""},
		Amount:                 int32(in.AmountPaise),
		TrialEndsAt:            tea,
	})
	if err != nil {
		return nil, err
	}
	_, _ = s.UpdateTenantPlan(ctx, in.TenantID, in.Plan, in.Status, in.TrialEndsAt)
	return &row, nil
}

func (s *Service) ListPlatformSubscriptions(ctx context.Context, limit, offset int32) ([]db.ListPlatformSubscriptionsRow, error) {
	return s.q.ListPlatformSubscriptions(ctx, db.ListPlatformSubscriptionsParams{Limit: limit, Offset: offset})
}

// GetFeatures returns a tenant's feature-flag JSON. Empty `{}` if no row.
func (s *Service) GetFeatures(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	raw, err := s.q.GetTenantFeatures(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return []byte("{}"), nil
	}
	return raw, nil
}

// SetFeatures replaces the feature-flag JSON for a tenant. The /super UI
// uses this to flip live/store/tests/ai_doubts/downloads on or off per
// tenant without code changes.
func (s *Service) SetFeatures(ctx context.Context, tenantID uuid.UUID, features []byte) ([]byte, error) {
	if len(features) == 0 {
		features = []byte("{}")
	}
	row, err := s.q.UpsertTenantFeatures(ctx, db.UpsertTenantFeaturesParams{
		TenantID: utils.UUIDToPg(tenantID),
		Features: features,
	})
	if err != nil {
		return nil, err
	}
	return row.Features, nil
}

// SetRazorpayAccount stores a tenant's Linked-Account ID so future course
// purchases auto-split via Razorpay Route. Pass an empty string to detach
// (rare — usually only when KYC has been revoked).
func (s *Service) SetRazorpayAccount(ctx context.Context, id uuid.UUID, accountID string) (*db.Tenant, error) {
	t, err := s.q.SetTenantRazorpayAccount(ctx, db.SetTenantRazorpayAccountParams{
		ID:      utils.UUIDToPg(id),
		Column2: accountID,
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ImpersonationResult is what the support tool gets back: a short-lived
// access token signed for the tenant_admin user inside the target tenant,
// plus enough metadata to render the "you are impersonating" banner.
type ImpersonationResult struct {
	AccessToken string    `json:"access_token"`
	TenantID    uuid.UUID `json:"tenant_id"`
	TenantName  string    `json:"tenant_name"`
	OrgCode     string    `json:"org_code"`
	UserID      uuid.UUID `json:"user_id"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Impersonate mints an access token for the tenant's owner (or first admin)
// so platform support can drop into the tenant's portal without their
// password. The token is signed with the same JWT secret as regular auth,
// but is short-lived (15m) and labelled with role=admin tied to the target
// tenant — students/instructors never get this kind of token.
//
// Caller is responsible for guarding this endpoint behind super_admin role
// + audit log; the audit row gets written by the middleware automatically
// thanks to the standard mutating-route capture.
func (s *Service) Impersonate(ctx context.Context, tenantID uuid.UUID, jwtSecret string) (*ImpersonationResult, error) {
	t, err := s.q.GetTenantByID(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return nil, err
	}

	// Pick a target user: tenant.owner_user_id if set, otherwise the first
	// active admin in tenant_users. Falls back to error if neither exists —
	// support can ask the tenant to create an admin first instead of us
	// minting a token bound to a nonexistent user.
	var ownerID uuid.UUID
	if t.OwnerUserID.Valid {
		ownerID = uuid.UUID(t.OwnerUserID.Bytes)
	} else {
		users, e := s.q.ListUsersForTenant(ctx, db.ListUsersForTenantParams{
			TenantID: utils.UUIDToPg(tenantID),
			Limit:    1,
			Offset:   0,
		})
		if e != nil || len(users) == 0 {
			return nil, fmt.Errorf("no admin user in tenant %s", tenantID)
		}
		ownerID = uuid.UUID(users[0].ID.Bytes)
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	tok, err := utils.GenerateAccessToken(ownerID, t.Name+"@impersonated", "admin",
		tenantID, jwtSecret, time.Until(expiresAt))
	if err != nil {
		return nil, err
	}
	return &ImpersonationResult{
		AccessToken: tok,
		TenantID:    tenantID,
		TenantName:  t.Name,
		OrgCode:     t.OrgCode,
		UserID:      ownerID,
		ExpiresAt:   expiresAt,
	}, nil
}

// UpdateLeadStatus is called from the leads triage view as the prospect
// moves through new → contacted → demo → won/lost.
func (s *Service) UpdateLeadStatus(ctx context.Context, id uuid.UUID, status, notes string, assignedTo *uuid.UUID) (*db.Lead, error) {
	assigned := pgtype.UUID{}
	if assignedTo != nil {
		assigned = pgtype.UUID{Bytes: *assignedTo, Valid: true}
	}
	row, err := s.q.UpdateLeadStatus(ctx, db.UpdateLeadStatusParams{
		ID:         utils.UUIDToPg(id),
		Status:     pgtype.Text{String: status, Valid: status != ""},
		Column3:    notes,
		AssignedTo: assigned,
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}
