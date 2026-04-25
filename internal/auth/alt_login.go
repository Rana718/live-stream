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

	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Dev-mode constants. Until we wire Firebase Auth (phase 2b) every OTP the
// server issues is ignored in favour of this canned value so the mobile app
// can still drive the full login flow end-to-end against localhost. Flip the
// constant to false before shipping to production or the stub will still
// accept "123456" as a valid code.
const (
	devModeOTP = true
	devOTPCode = "123456"
)

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
// Returning the dev code is a debug affordance — flip devModeOTP off and wire
// MSG91 in production.
func (s *Service) SendOTP(ctx context.Context, phoneInput, orgCode string) (phone, devCode string, err error) {
	phone, err = normalizePhone(phoneInput)
	if err != nil {
		return "", "", err
	}

	// We don't strictly *need* the tenant at this point (the SMS code is
	// keyed on phone + a hash) but resolving it now gives us a fast,
	// pre-send "is this org real" check so a typo doesn't burn an SMS.
	if _, err := s.resolveTenant(ctx, orgCode); err != nil {
		return "", "", err
	}

	if err := s.queries.InvalidateOlderSmsCodes(ctx, phone); err != nil {
		return "", "", err
	}

	code := devOTPCode
	if !devModeOTP {
		code, err = random6DigitCode()
		if err != nil {
			return "", "", err
		}
	}

	_, err = s.queries.CreateSmsCode(ctx, db.CreateSmsCodeParams{
		PhoneNumber: phone,
		CodeHash:    hashCode(code),
		ExpiresAt:   pgtype.Timestamp{Time: time.Now().Add(5 * time.Minute), Valid: true},
	})
	if err != nil {
		return "", "", err
	}

	// Dispatch via SMS provider (MSG91). In dev we short-circuit and return
	// the code so the QA flow can skip the SMS leg.
	if !devModeOTP && s.sms != nil {
		if e := s.sms.SendOTP(ctx, phone, code); e != nil {
			return "", "", fmt.Errorf("sms send failed: %w", e)
		}
	}
	if devModeOTP {
		return phone, code, nil
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
// the resolved tenant.
func (s *Service) LoginWithOTP(ctx context.Context, phone, code, orgCode string) (*TokenResponse, error) {
	user, tenantID, err := s.VerifyOTP(ctx, phone, code, orgCode)
	if err != nil {
		return nil, err
	}
	return s.issueTokensForUser(ctx, user, tenantID)
}

// GoogleIdentity is the minimum data we accept from the client after a Google
// Sign-In round-trip. Once Firebase Admin SDK verification lands (phase 2b)
// this will come from decoding the ID token rather than trusting the client.
type GoogleIdentity struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
}

// LoginWithGoogle creates-or-fetches a user keyed by the Google subject claim
// inside the resolved tenant, then issues tokens. The tenant is required so
// the same Google account can belong to different orgs as separate user rows.
func (s *Service) LoginWithGoogle(ctx context.Context, id GoogleIdentity, orgCode string) (*TokenResponse, error) {
	if id.Sub == "" || id.Email == "" {
		return nil, fmt.Errorf("missing google identity")
	}

	tenantID, err := s.resolveTenant(ctx, orgCode)
	if err != nil {
		return nil, err
	}
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

// LinkGoogle attaches a Google identity to an existing authenticated account.
// Mirrors LinkPhone's conflict handling.
func (s *Service) LinkGoogle(ctx context.Context, userID, tenantID uuid.UUID, id GoogleIdentity) (*db.User, error) {
	if id.Sub == "" {
		return nil, fmt.Errorf("missing google sub")
	}
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
