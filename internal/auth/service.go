// Package auth implements phone-OTP and Google sign-in against the schema-v2
// identity tables (users / auth_identities / tenant_users / refresh_tokens).
// A user is one global identity; their role is per-tenant (tenant_users) and
// the access token is minted for one chosen tenant. Refresh tokens are
// opaque, DB-backed and family-rotated (reuse revokes the whole family).
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"live-platform/internal/auth/google"
	"live-platform/internal/config"
	"live-platform/internal/database"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// SMSClient is the minimal contract auth needs to deliver an OTP.
type SMSClient interface {
	SendOTP(ctx context.Context, phone, code string) error
}

// Referrer lets OTP/Google signup attach a referral code. Wired in Phase F;
// nil is a no-op.
type Referrer interface {
	AttachSignup(ctx context.Context, tenantID, newUserID uuid.UUID, code string)
}

type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	redis    *redis.Client
	cfg      *config.Config
	sms      SMSClient
	google   *google.Verifier
	referrer Referrer
}

func NewService(pool *pgxpool.Pool, rdb *redis.Client, cfg *config.Config) *Service {
	return &Service{pool: pool, q: db.New(pool), redis: rdb, cfg: cfg}
}

func (s *Service) WithSMS(c SMSClient) *Service           { s.sms = c; return s }
func (s *Service) WithGoogle(v *google.Verifier) *Service { s.google = v; return s }
func (s *Service) WithReferrer(r Referrer) *Service       { s.referrer = r; return s }

var (
	ErrInvalidOrgCode   = errors.New("invalid org code")
	ErrOTPNotConfigured = errors.New("otp delivery is not configured")
	ErrOTPThrottled     = errors.New("too many OTP requests — try again later")
	ErrInvalidCode      = errors.New("invalid or expired code")
	ErrTooManyAttempts  = errors.New("too many attempts — request a new code")
	ErrNoMembership     = errors.New("user is not a member of this org")
	ErrGoogleDisabled   = errors.New("google sign-in is not configured")
)

// ---- config helpers -------------------------------------------------------

func (s *Service) accessTTL() time.Duration {
	d, err := time.ParseDuration(s.cfg.JWT.AccessExpiry)
	if err != nil || d <= 0 {
		return 15 * time.Minute
	}
	return d
}

func (s *Service) refreshTTL() time.Duration {
	d, err := time.ParseDuration(s.cfg.JWT.RefreshExpiry)
	if err != nil || d <= 0 {
		return 7 * 24 * time.Hour
	}
	return d
}

func (s *Service) otpTTL() time.Duration {
	if s.cfg.OTP.TTLSec > 0 {
		return time.Duration(s.cfg.OTP.TTLSec) * time.Second
	}
	return 5 * time.Minute
}

func (s *Service) otpMaxSendsPerHour() int {
	if s.cfg.OTP.MaxSends > 0 {
		return s.cfg.OTP.MaxSends
	}
	return 5
}

func (s *Service) devCode() string {
	if s.cfg.OTP.DevCode != "" {
		return s.cfg.OTP.DevCode
	}
	return "123456"
}

// ---- tenant resolution --------------------------------------------------

// DefaultOrgCode is used in dev when a client omits org_code.
const DefaultOrgCode = "DEMO"

func (s *Service) resolveTenant(ctx context.Context, orgCode string) (db.GetTenantByOrgCodeRow, error) {
	code := strings.TrimSpace(orgCode)
	if code == "" {
		code = DefaultOrgCode
	}
	row, err := s.q.GetTenantByOrgCode(database.WithPublicLookup(ctx), code)
	if err != nil {
		return db.GetTenantByOrgCodeRow{}, ErrInvalidOrgCode
	}
	return row, nil
}

// ---- token bundle -----------------------------------------------------------

type UserInfo struct {
	ID                   uuid.UUID `json:"id"`
	Email                string    `json:"email,omitempty"`
	Phone                string    `json:"phone,omitempty"`
	FullName             string    `json:"full_name,omitempty"`
	Role                 string    `json:"role"`
	TenantID             uuid.UUID `json:"tenant_id"`
	IsPlatformSuperAdmin bool      `json:"is_platform_super_admin,omitempty"`
}

type TokenBundle struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int      `json:"expires_in"`
	User         UserInfo `json:"user"`
}

// issueTokens resolves the user's role for tenantID, mints an access token,
// and creates a fresh refresh-token row (optionally chained to parentID for
// rotation). Runs cross-tenant (WithSuperAdmin) — this is a trusted server op
// and the user is not yet request-authenticated.
func (s *Service) issueTokens(ctx context.Context, userID, tenantID uuid.UUID, parentID, familyID uuid.UUID, userAgent, ip string) (*TokenBundle, error) {
	sctx := database.WithSuperAdmin(ctx)

	u, err := s.q.GetUserByID(sctx, pgUUID(userID))
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	mem, err := s.q.GetTenantUser(sctx, db.GetTenantUserParams{TenantID: pgUUID(tenantID), UserID: pgUUID(userID)})
	if err != nil {
		return nil, ErrNoMembership
	}
	role := string(mem.Role)
	// A platform super-admin is super_admin in every tenant they hold a
	// membership in — the tenant_users row only exists so OTP login can
	// resolve a tenant. SuperAdminContext / RequireRole key off this string.
	if u.IsPlatformSuperAdmin {
		role = "super_admin"
	}

	access, err := utils.GenerateAccessToken(userID, textVal(u.Email), role, tenantID, u.TokenVersion, s.cfg.JWT.AccessSecret, s.accessTTL())
	if err != nil {
		return nil, err
	}

	raw, hash, err := utils.NewOpaqueToken()
	if err != nil {
		return nil, err
	}
	if familyID == uuid.Nil {
		familyID = uuid.New()
	}
	_, err = s.q.CreateRefreshToken(sctx, db.CreateRefreshTokenParams{
		UserID:    pgUUID(userID),
		TenantID:  pgUUID(tenantID),
		FamilyID:  pgUUID(familyID),
		ParentID:  pgUUIDOrNull(parentID),
		TokenHash: hash,
		UserAgent: textOrNull(userAgent),
		Ip:        inetOrNull(ip),
		ExpiresAt: pgTime(time.Now().Add(s.refreshTTL())),
	})
	if err != nil {
		return nil, err
	}
	_ = s.q.TouchUserLastLogin(sctx, pgUUID(userID))

	return &TokenBundle{
		AccessToken:  access,
		RefreshToken: raw,
		ExpiresIn:    int(s.accessTTL().Seconds()),
		User: UserInfo{
			ID: userID, Email: textVal(u.Email), Phone: textVal(u.Phone),
			FullName: textVal(u.FullName), Role: role, TenantID: tenantID,
			IsPlatformSuperAdmin: u.IsPlatformSuperAdmin,
		},
	}, nil
}

// Refresh rotates a refresh token. Reuse of an already-used token revokes
// the whole family (breach response).
func (s *Service) Refresh(ctx context.Context, rawToken, userAgent, ip string) (*TokenBundle, error) {
	sctx := database.WithSuperAdmin(ctx)
	row, err := s.q.GetRefreshTokenByHash(sctx, utils.HashToken(rawToken))
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}
	if row.RevokedAt.Valid {
		return nil, errors.New("refresh token revoked")
	}
	if row.ExpiresAt.Time.Before(time.Now()) {
		return nil, errors.New("refresh token expired")
	}
	if row.UsedAt.Valid {
		// Reuse detected — burn the family.
		_ = s.q.RevokeRefreshTokenFamily(sctx, row.FamilyID)
		return nil, errors.New("refresh token already used — session revoked")
	}

	userID := uuid.UUID(row.UserID.Bytes)
	tenantID := uuid.UUID(row.TenantID.Bytes)
	bundle, err := s.issueTokens(ctx, userID, tenantID, uuid.UUID(row.ID.Bytes), uuid.UUID(row.FamilyID.Bytes), userAgent, ip)
	if err != nil {
		return nil, err
	}
	// Mark the old token used and link it to its replacement.
	newRow, _ := s.q.GetRefreshTokenByHash(sctx, utils.HashToken(bundle.RefreshToken))
	_ = s.q.MarkRefreshTokenUsed(sctx, db.MarkRefreshTokenUsedParams{ID: row.ID, ReplacedBy: newRow.ID})
	return bundle, nil
}

// Logout revokes the presented refresh token's whole family.
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	sctx := database.WithSuperAdmin(ctx)
	row, err := s.q.GetRefreshTokenByHash(sctx, utils.HashToken(rawToken))
	if err != nil {
		return nil // already gone
	}
	return s.q.RevokeRefreshTokenFamily(sctx, row.FamilyID)
}

// SwitchOrg re-mints tokens for a different tenant the user belongs to.
func (s *Service) SwitchOrg(ctx context.Context, userID, targetTenantID uuid.UUID, userAgent, ip string) (*TokenBundle, error) {
	return s.issueTokens(ctx, userID, targetTenantID, uuid.Nil, uuid.Nil, userAgent, ip)
}

// tx helper
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

var _ = pgx.ErrNoRows

// ---- pgtype helpers (local, money-free) ---------------------------------

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }
func pgUUIDOrNull(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}
func textVal(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}
func textOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
func pgTime(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func inetOrNull(s string) *netip.Addr {
	if s == "" {
		return nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return nil
	}
	return &addr
}
