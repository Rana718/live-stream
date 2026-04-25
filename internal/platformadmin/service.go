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
