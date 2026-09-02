// Package tenants is the multi-tenant control plane: public org-code /
// custom-domain resolution, tenant branding + feature flags, self-serve
// onboarding, and super-admin provisioning. Schema v2.
package tenants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"live-platform/internal/database"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, q: db.New(pool)} }

// ---- public lookup -----------------------------------------------------

type PublicTenantInfo struct {
	ID      uuid.UUID       `json:"id"`
	OrgCode string          `json:"org_code"`
	Name    string          `json:"name"`
	Slug    string          `json:"slug"`
	LogoURL string          `json:"logo_url"`
	Theme   json.RawMessage `json:"theme"`
	Status  string          `json:"status"`
}

func (s *Service) LookupByOrgCode(ctx context.Context, code string) (*PublicTenantInfo, error) {
	row, err := s.q.GetTenantByOrgCode(database.WithPublicLookup(ctx), strings.TrimSpace(code))
	if err != nil {
		return nil, errors.New("org not found")
	}
	return &PublicTenantInfo{
		ID: uuid.UUID(row.ID.Bytes), OrgCode: row.OrgCode, Name: row.Name, Slug: row.Slug,
		LogoURL: utils.TextFromPg(row.LogoUrl), Theme: json.RawMessage(row.Theme),
		Status: string(row.Status),
	}, nil
}

func (s *Service) DomainIsRegistered(ctx context.Context, domain string) bool {
	_, err := s.q.GetTenantByDomain(database.WithPublicLookup(ctx), strings.ToLower(strings.TrimSpace(domain)))
	return err == nil
}

// ---- authenticated tenant reads --------------------------------------

func (s *Service) MyTenant(ctx context.Context, tenantID uuid.UUID) (map[string]any, error) {
	t, err := s.q.GetTenantByID(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return nil, errors.New("tenant not found")
	}
	return map[string]any{
		"id": tenantID, "org_code": t.OrgCode, "name": t.Name, "slug": t.Slug,
		"status": string(t.Status), "plan": string(t.Plan),
		"logo_url": utils.TextFromPg(t.LogoUrl), "theme": json.RawMessage(t.Theme),
		"legal_name": utils.TextFromPg(t.LegalName), "gstin": utils.TextFromPg(t.Gstin),
		"place_of_supply": utils.TextFromPg(t.PlaceOfSupply), "timezone": t.Timezone,
		"trial_ends_at": tsPtr(t.TrialEndsAt),
	}, nil
}

func (s *Service) GetFeatures(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	row, err := s.q.GetTenantSettings(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return []byte(`{}`), nil
	}
	if len(row.Features) == 0 {
		return []byte(`{}`), nil
	}
	return row.Features, nil
}

// ---- branding --------------------------------------------------------

type UpdateBrandingRequest struct {
	Name    string          `json:"name"`
	LogoURL string          `json:"logo_url"`
	Theme   json.RawMessage `json:"theme"`
}

func (s *Service) UpdateBranding(ctx context.Context, tenantID uuid.UUID, req UpdateBrandingRequest) (map[string]any, error) {
	var theme []byte
	if len(req.Theme) > 0 {
		theme = req.Theme
	}
	_, err := s.q.UpdateTenantBranding(ctx, db.UpdateTenantBrandingParams{
		ID:      utils.UUIDToPg(tenantID),
		Name:    utils.TextToPg(req.Name),
		LogoUrl: utils.TextToPg(req.LogoURL),
		Theme:   theme,
	})
	if err != nil {
		return nil, err
	}
	return s.MyTenant(ctx, tenantID)
}

// ---- self-serve onboarding -----------------------------------------

type SelfServeOnboardRequest struct {
	OrgName    string `json:"org_name"`
	AdminName  string `json:"admin_name"`
	AdminPhone string `json:"admin_phone"`
	AdminEmail string `json:"admin_email"`
	City       string `json:"city"`
}

type SelfServeOnboardResult struct {
	TenantID uuid.UUID `json:"tenant_id"`
	OrgCode  string    `json:"org_code"`
	Slug     string    `json:"slug"`
	AdminID  uuid.UUID `json:"admin_id"`
}

func (s *Service) SelfServeOnboard(ctx context.Context, req SelfServeOnboardRequest) (*SelfServeOnboardResult, error) {
	phone := strings.ReplaceAll(strings.TrimSpace(req.AdminPhone), " ", "")
	orgCode := letterPrefix(req.OrgName, 4) + fmt.Sprintf("%04d", randomFourDigit())
	slug := slugify(req.OrgName)
	if slug == "" {
		slug = strings.ToLower(orgCode)
	}

	sctx := database.WithSuperAdmin(ctx)
	res := &SelfServeOnboardResult{OrgCode: orgCode, Slug: slug}

	err := s.inTx(sctx, func(q *db.Queries) error {
		// First admin user (global identity + phone identity).
		u, err := q.CreateUser(sctx, db.CreateUserParams{
			Phone:    utils.TextToPg(phone),
			Email:    utils.TextToPg(req.AdminEmail),
			FullName: utils.TextToPg(req.AdminName),
		})
		if err != nil {
			return fmt.Errorf("create admin: %w", err)
		}
		res.AdminID = uuid.UUID(u.ID.Bytes)
		if _, err := q.CreateAuthIdentity(sctx, db.CreateAuthIdentityParams{
			UserID: u.ID, Provider: "phone", ProviderUid: phone,
		}); err != nil {
			return fmt.Errorf("admin identity: %w", err)
		}

		t, err := q.CreateTenant(sctx, db.CreateTenantParams{
			OrgCode:     orgCode,
			Name:        req.OrgName,
			Slug:        slug,
			Plan:        db.NullTenantPlan{TenantPlan: db.TenantPlanStarter, Valid: true},
			Status:      db.NullTenantStatus{TenantStatus: db.TenantStatusTrial, Valid: true},
			OwnerUserID: u.ID,
		})
		if err != nil {
			return fmt.Errorf("create tenant: %w", err)
		}
		res.TenantID = uuid.UUID(t.ID.Bytes)

		if _, err := q.UpsertTenantSettings(sctx, db.UpsertTenantSettingsParams{
			TenantID: t.ID,
			Features: []byte(`{"live":true,"store":true,"tests":true,"ai_doubts":true,"downloads":true}`),
		}); err != nil {
			return fmt.Errorf("tenant settings: %w", err)
		}
		if _, err := q.AddTenantUser(sctx, db.AddTenantUserParams{
			TenantID: t.ID, UserID: u.ID, Role: db.TenantRoleOwner,
		}); err != nil {
			return fmt.Errorf("owner membership: %w", err)
		}
		// 14-day trial clock.
		if err := q.SetTenantStatus(sctx, db.SetTenantStatusParams{ID: t.ID, Status: db.TenantStatusTrial}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Lead row for sales follow-up (best-effort).
	_, _ = s.q.CreateLead(sctx, db.CreateLeadParams{
		Name: utils.TextToPg(req.AdminName), Phone: utils.TextToPg(phone),
		Email: utils.TextToPg(req.AdminEmail), InstituteName: utils.TextToPg(req.OrgName),
		City: utils.TextToPg(req.City), Source: utils.TextToPg("self_serve"),
	})
	return res, nil
}

// ---- super-admin provisioning ------------------------------------

type CreateTenantRequest struct {
	OrgCode       string `json:"org_code"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Plan          string `json:"plan"`
	PlaceOfSupply string `json:"place_of_supply"`
}

func (s *Service) Create(ctx context.Context, req CreateTenantRequest, ownerID uuid.UUID) (map[string]any, error) {
	sctx := database.WithSuperAdmin(ctx)
	code := strings.ToUpper(strings.TrimSpace(req.OrgCode))
	if code == "" {
		code = letterPrefix(req.Name, 4) + fmt.Sprintf("%04d", randomFourDigit())
	}
	slug := req.Slug
	if slug == "" {
		slug = slugify(req.Name)
	}
	plan := db.NullTenantPlan{}
	if req.Plan != "" {
		plan = db.NullTenantPlan{TenantPlan: db.TenantPlan(req.Plan), Valid: true}
	}
	t, err := s.q.CreateTenant(sctx, db.CreateTenantParams{
		OrgCode:       code,
		Name:          req.Name,
		Slug:          slug,
		Plan:          plan,
		PlaceOfSupply: utils.TextToPg(req.PlaceOfSupply),
		OwnerUserID:   pgtype.UUID{Bytes: ownerID, Valid: ownerID != uuid.Nil},
	})
	if err != nil {
		return nil, err
	}
	_, _ = s.q.UpsertTenantSettings(sctx, db.UpsertTenantSettingsParams{TenantID: t.ID})
	return map[string]any{
		"id": uuid.UUID(t.ID.Bytes), "org_code": t.OrgCode, "name": t.Name,
		"slug": t.Slug, "status": string(t.Status), "plan": string(t.Plan),
	}, nil
}

// ---- helpers ---------------------------------------------------------

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
