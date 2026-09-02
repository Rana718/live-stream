package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"live-platform/internal/auth/google"
	"live-platform/internal/database"
	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// GoogleIdentity is the internal, already-verified identity passed between
// helpers in this file. It is never populated from a client payload —
// verifyGoogle is the only source.
type GoogleIdentity struct {
	Sub      string
	Email    string
	FullName string
}

// OTP dev-mode is controlled by config (OTP_DEV_MODE, default false).
// When on, SendOTP skips real SMS and stores the configured dev code so QA
// can drive the flow against localhost. config.validate() refuses to boot
// production with dev-mode enabled.
func (s *Service) otpDevMode() bool { return s.cfg != nil && s.cfg.OTP.DevMode }

func (s *Service) otpDevCode() string {
	if s.cfg != nil && s.cfg.OTP.DevCode != "" {
		return s.cfg.OTP.DevCode
	}
	return "123456"
}

func (s *Service) otpTTL() time.Duration {
	if s.cfg != nil && s.cfg.OTP.TTLSec > 0 {
		return time.Duration(s.cfg.OTP.TTLSec) * time.Second
	}
	return 5 * time.Minute
}

func (s *Service) otpMaxSendsPerHour() int {
	if s.cfg != nil && s.cfg.OTP.MaxSends > 0 {
		return s.cfg.OTP.MaxSends
	}
	return 5
}

// E.164-ish check — good enough to reject obvious garbage without locking
// out domestic-only numbers. A real integration would normalize via libphonenumber.
var phoneRegex = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

func normalizePhone(raw string) (string, error) {
	p := strings.ReplaceAll(strings.TrimSpace(raw), " ", "")
	p = strings.ReplaceAll(p, "-", "")
	if !phoneRegex.MatchString(p) {
		return "", fmt.Errorf("invalid phone number")
	}
	return p, nil
}

func hashCode(code string) string {
	h := sha256.Sum256([]byte(code))
	return hex.EncodeToString(h[:])
}

func random6DigitCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// SendOTP issues a fresh code for `phone`. The Org Code is captured here too
// so the eventual VerifyOTP knows which tenant to scope the new user under.
// devCode is only ever non-empty when OTP_DEV_MODE is on (never in prod).
func (s *Service) SendOTP(ctx context.Context, phoneInput, orgCode string) (phone, devCode string, err error) {
	phone, err = normalizePhone(phoneInput)
	if err != nil {
		return "", "", err
	}

	// Real SMS delivery must be wired unless we're explicitly in dev mode —
	// otherwise a caller would get "sent: true" and never receive a code.
	if !s.otpDevMode() && s.sms == nil {
		return "", "", fmt.Errorf("otp delivery is not configured")
	}

	// We don't strictly *need* the tenant at this point (the SMS code is
	// keyed on phone + a hash) but resolving it now gives us a fast,
	// pre-send "is this org real" check so a typo doesn't burn an SMS.
	if _, err := s.resolveTenant(ctx, orgCode); err != nil {
		return "", "", err
	}

	// Per-phone send throttle — stops SMS-bombing / cost abuse. Backed by
	// Redis so it holds across instances. Fails open if Redis is down
	// (availability > perfect rate limiting for the login path).
	if s.redis != nil {
		key := "otp:sends:" + phone
		n, incErr := s.redis.Incr(ctx, key).Result()
		if incErr == nil {
			if n == 1 {
				_ = s.redis.Expire(ctx, key, time.Hour).Err()
			}
			if int(n) > s.otpMaxSendsPerHour() {
				return "", "", fmt.Errorf("too many OTP requests — try again later")
			}
		}
	}

	if err := s.queries.InvalidateOlderSmsCodes(ctx, phone); err != nil {
		return "", "", err
	}

	code := s.otpDevCode()
	if !s.otpDevMode() {
		code, err = random6DigitCode()
		if err != nil {
			return "", "", err
		}
	}

	_, err = s.queries.CreateSmsCode(ctx, db.CreateSmsCodeParams{
		PhoneNumber: phone,
		CodeHash:    hashCode(code),
		ExpiresAt:   pgtype.Timestamp{Time: time.Now().Add(s.otpTTL()), Valid: true},
	})
	if err != nil {
		return "", "", err
	}

	if s.otpDevMode() {
		return phone, code, nil
	}
	if e := s.sms.SendOTP(ctx, phone, code); e != nil {
		return "", "", fmt.Errorf("sms send failed: %w", e)
	}
	return phone, "", nil
}

// VerifyOTP consumes a pending code, returning the matching user (creating one
// if the phone is new). Tenant-scoped: the user is looked up / created within
// the tenant resolved from orgCode.
func (s *Service) VerifyOTP(ctx context.Context, phoneInput, code, orgCode string) (*db.User, uuid.UUID, error) {
	phone, err := normalizePhone(phoneInput)
	if err != nil {
		return nil, uuid.Nil, err
	}

	tenantID, err := s.resolveTenant(ctx, orgCode)
	if err != nil {
		return nil, uuid.Nil, err
	}
	// resolveTenant only proved the org code is real; every query below
	// touches tenant-scoped tables (RLS-forced), so the connection needs
	// app.tenant_id set to the tenant we just resolved.
	ctx = database.WithTenant(ctx, tenantID.String(), "")

	row, err := s.queries.GetLatestSmsCode(ctx, phone)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("no active code for this number")
	}
	if row.Attempts.Int32 >= 5 {
		return nil, uuid.Nil, fmt.Errorf("too many attempts — request a new code")
	}
	if row.CodeHash != hashCode(code) {
		_ = s.queries.IncrementSmsCodeAttempts(ctx, row.ID)
		return nil, uuid.Nil, fmt.Errorf("invalid code")
	}
	if err := s.queries.ConsumeSmsCode(ctx, row.ID); err != nil {
		return nil, uuid.Nil, err
	}

	// Existing user on this phone in this tenant?
	if user, err := s.queries.GetUserByPhone(ctx, db.GetUserByPhoneParams{
		TenantID:    pgtype.UUID{Bytes: tenantID, Valid: true},
		PhoneNumber: pgtype.Text{String: phone, Valid: true},
	}); err == nil {
		return &user, tenantID, nil
	}

	// First time we see this phone in this tenant — auto-provision a student
	// account. No username, no email, no password — phone is the identity.
	user, err := s.queries.CreateUserWithPhone(ctx, db.CreateUserWithPhoneParams{
		TenantID:    pgtype.UUID{Bytes: tenantID, Valid: true},
		PhoneNumber: pgtype.Text{String: phone, Valid: true},
	})
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("couldn't create account: %w", err)
	}

	// Ensure tenant_users membership exists for the freshly created student.
	_, _ = s.queries.AddTenantUser(ctx, db.AddTenantUserParams{
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
		UserID:   user.ID,
		Role:     "student",
	})
	return &user, tenantID, nil
}

// LoginWithOTP is the handler-facing path: verify + issue tokens scoped to
// the resolved tenant. If `referralCode` is non-empty AND this verify call
// freshly created the user, the referrer gets credited via the injected
// referrals service. Best-effort — bad codes never fail login.
func (s *Service) LoginWithOTP(ctx context.Context, phone, code, orgCode, referralCode string) (*TokenResponse, error) {
	wasNew := false
	{
		// Pre-check: did a row already exist for this phone in this tenant?
		// Used to decide whether to fire the referral attach. We can't read
		// it after VerifyOTP because that path returns the same row whether
		// it auto-created or not.
		phoneNorm, _ := normalizePhone(phone)
		tID, _ := s.resolveTenant(ctx, orgCode)
		if tID != uuid.Nil {
			preCheckCtx := database.WithTenant(ctx, tID.String(), "")
			_, err := s.queries.GetUserByPhone(preCheckCtx, db.GetUserByPhoneParams{
				TenantID:    pgtype.UUID{Bytes: tID, Valid: true},
				PhoneNumber: pgtype.Text{String: phoneNorm, Valid: phoneNorm != ""},
			})
			wasNew = err != nil
		}
	}

	user, tenantID, err := s.VerifyOTP(ctx, phone, code, orgCode)
	if err != nil {
		return nil, err
	}

	if wasNew && s.referrer != nil && referralCode != "" {
		s.referrer.AttachToSignup(ctx, tenantID, uuid.UUID(user.ID.Bytes), referralCode)
	}

	return s.issueTokensForUser(ctx, user, tenantID)
}

// GoogleCredential is what the client sends after a Google Sign-In
// round-trip: the raw ID token (a signed JWT). We verify it server-side —
// identity fields are never taken from the client directly.
type GoogleCredential struct {
	IDToken string `json:"id_token"`
	// Deprecated client fields — accepted in the payload for backwards
	// compatibility but ignored. The trusted identity always comes from
	// verifying IDToken.
	Sub      string `json:"sub,omitempty"`
	Email    string `json:"email,omitempty"`
	FullName string `json:"full_name,omitempty"`
}

// verifyGoogle turns a raw ID token into a trusted identity, or errors.
func (s *Service) verifyGoogle(ctx context.Context, idToken string) (*google.Identity, error) {
	if s.google == nil || !s.google.Enabled() {
		return nil, fmt.Errorf("google sign-in is not configured")
	}
	if idToken == "" {
		return nil, fmt.Errorf("missing google id_token")
	}
	id, err := s.google.Verify(ctx, idToken)
	if err != nil {
		return nil, err
	}
	if !id.EmailVerified {
		return nil, fmt.Errorf("google account email is not verified")
	}
	return id, nil
}

// LoginWithGoogle verifies the ID token then creates-or-fetches a user keyed
// by the Google subject claim inside the resolved tenant, then issues tokens.
func (s *Service) LoginWithGoogle(ctx context.Context, cred GoogleCredential, orgCode string) (*TokenResponse, error) {
	verified, err := s.verifyGoogle(ctx, cred.IDToken)
	if err != nil {
		return nil, err
	}
	id := GoogleIdentity{Sub: verified.Sub, Email: verified.Email, FullName: verified.FullName}

	tenantID, err := s.resolveTenant(ctx, orgCode)
	if err != nil {
		return nil, err
	}
	ctx = database.WithTenant(ctx, tenantID.String(), "")
	tID := pgtype.UUID{Bytes: tenantID, Valid: true}

	// Prefer google_sub within tenant so email rotations don't lose the link.
	if user, err := s.queries.GetUserByGoogleSub(ctx, db.GetUserByGoogleSubParams{
		TenantID:  tID,
		GoogleSub: pgtype.Text{String: id.Sub, Valid: true},
	}); err == nil {
		return s.issueTokensForUser(ctx, &user, tenantID)
	}

	// Existing account on the same email in this tenant? Attach google_sub.
	if user, err := s.queries.GetUserByEmail(ctx, db.GetUserByEmailParams{
		TenantID: tID,
		Lower:    id.Email,
	}); err == nil {
		linked, err := s.queries.LinkGoogleToUser(ctx, db.LinkGoogleToUserParams{
			ID:        user.ID,
			GoogleSub: pgtype.Text{String: id.Sub, Valid: true},
		})
		if err != nil {
			return nil, err
		}
		return s.issueTokensForUser(ctx, &linked, tenantID)
	}

	// Brand-new account from Google. No username — phone-or-email-only.
	user, err := s.queries.CreateUserWithGoogle(ctx, db.CreateUserWithGoogleParams{
		TenantID:  tID,
		Email:     pgtype.Text{String: id.Email, Valid: true},
		FullName:  pgtype.Text{String: id.FullName, Valid: id.FullName != ""},
		GoogleSub: pgtype.Text{String: id.Sub, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	_, _ = s.queries.AddTenantUser(ctx, db.AddTenantUserParams{
		TenantID: tID,
		UserID:   user.ID,
		Role:     "student",
	})
	return s.issueTokensForUser(ctx, &user, tenantID)
}

// LinkPhone attaches a verified phone number to an existing authenticated
// account. Tenant-scoped: links happen within the user's current tenant only.
func (s *Service) LinkPhone(ctx context.Context, userID uuid.UUID, tenantID uuid.UUID, phoneInput, code, orgCode string) (*db.User, error) {
	phone, err := normalizePhone(phoneInput)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.VerifyOTP(ctx, phone, code, orgCode); err != nil {
		return nil, err
	}

	tID := pgtype.UUID{Bytes: tenantID, Valid: true}
	// Phone might already belong to a different user if the learner used OTP
	// login at some point in the past. Refuse rather than silently merge.
	if other, err := s.queries.GetUserByPhone(ctx, db.GetUserByPhoneParams{
		TenantID:    tID,
		PhoneNumber: pgtype.Text{String: phone, Valid: true},
	}); err == nil {
		if uuid.UUID(other.ID.Bytes) != userID {
			return nil, fmt.Errorf("phone already belongs to another account")
		}
		return &other, nil
	}

	linked, err := s.queries.LinkPhoneToUser(ctx, db.LinkPhoneToUserParams{
		ID:          pgtype.UUID{Bytes: userID, Valid: true},
		PhoneNumber: pgtype.Text{String: phone, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return &linked, nil
}

// LinkGoogle attaches a Google identity to an existing authenticated account
// after verifying the ID token. Mirrors LinkPhone's conflict handling.
func (s *Service) LinkGoogle(ctx context.Context, userID, tenantID uuid.UUID, cred GoogleCredential) (*db.User, error) {
	verified, err := s.verifyGoogle(ctx, cred.IDToken)
	if err != nil {
		return nil, err
	}
	id := GoogleIdentity{Sub: verified.Sub, Email: verified.Email, FullName: verified.FullName}
	tID := pgtype.UUID{Bytes: tenantID, Valid: true}
	if other, err := s.queries.GetUserByGoogleSub(ctx, db.GetUserByGoogleSubParams{
		TenantID:  tID,
		GoogleSub: pgtype.Text{String: id.Sub, Valid: true},
	}); err == nil {
		if uuid.UUID(other.ID.Bytes) != userID {
			return nil, fmt.Errorf("google account already linked elsewhere")
		}
		return &other, nil
	}
	linked, err := s.queries.LinkGoogleToUser(ctx, db.LinkGoogleToUserParams{
		ID:        pgtype.UUID{Bytes: userID, Valid: true},
		GoogleSub: pgtype.Text{String: id.Sub, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return &linked, nil
}
