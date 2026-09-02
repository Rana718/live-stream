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

	"live-platform/internal/database"
	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var phoneRE = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

func normalizePhone(raw string) (string, error) {
	p := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(raw), " ", ""), "-", "")
	if !phoneRE.MatchString(p) {
		return "", fmt.Errorf("invalid phone number")
	}
	return p, nil
}

func hashCode(code string) string {
	h := sha256.Sum256([]byte(code))
	return hex.EncodeToString(h[:])
}

func random6() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// SendOTP issues a login code for a phone. Returns devCode only when
// OTP_DEV_MODE is on. Per-phone hourly throttle via Redis (fails open).
func (s *Service) SendOTP(ctx context.Context, phoneInput, orgCode string) (phone, devCode string, err error) {
	phone, err = normalizePhone(phoneInput)
	if err != nil {
		return "", "", err
	}
	if !s.cfg.OTP.DevMode && s.sms == nil {
		return "", "", ErrOTPNotConfigured
	}
	if _, err = s.resolveTenant(ctx, orgCode); err != nil {
		return "", "", err
	}

	if s.redis != nil {
		key := "otp:sends:" + phone
		if n, e := s.redis.Incr(ctx, key).Result(); e == nil {
			if n == 1 {
				_ = s.redis.Expire(ctx, key, time.Hour).Err()
			}
			if int(n) > s.otpMaxSendsPerHour() {
				return "", "", ErrOTPThrottled
			}
		}
	}

	sctx := database.WithSuperAdmin(ctx)
	_ = s.q.InvalidatePendingOtpCodes(sctx, db.InvalidatePendingOtpCodesParams{
		Destination: phone, Purpose: db.OtpPurposeLogin,
	})

	code := s.devCode()
	if !s.cfg.OTP.DevMode {
		if code, err = random6(); err != nil {
			return "", "", err
		}
	}
	if _, err = s.q.CreateOtpCode(sctx, db.CreateOtpCodeParams{
		Channel:     db.OtpChannelSms,
		Purpose:     db.OtpPurposeLogin,
		Destination: phone,
		CodeHash:    hashCode(code),
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(s.otpTTL()), Valid: true},
	}); err != nil {
		return "", "", err
	}

	if s.cfg.OTP.DevMode {
		return phone, code, nil
	}
	if e := s.sms.SendOTP(ctx, phone, code); e != nil {
		return "", "", fmt.Errorf("sms send failed: %w", e)
	}
	return phone, "", nil
}

// VerifyOTP consumes a code, finds-or-creates the user + tenant membership,
// and issues tokens.
func (s *Service) VerifyOTP(ctx context.Context, phoneInput, code, orgCode, referralCode, userAgent, ip string) (*TokenBundle, error) {
	phone, err := normalizePhone(phoneInput)
	if err != nil {
		return nil, err
	}
	tenant, err := s.resolveTenant(ctx, orgCode)
	if err != nil {
		return nil, err
	}
	tenantID := uuid.UUID(tenant.ID.Bytes)
	sctx := database.WithSuperAdmin(ctx)

	row, err := s.q.GetLatestOtpCode(sctx, db.GetLatestOtpCodeParams{Destination: phone, Purpose: db.OtpPurposeLogin})
	if err != nil {
		return nil, ErrInvalidCode
	}
	if row.ExpiresAt.Time.Before(time.Now()) {
		return nil, ErrInvalidCode
	}
	if row.Attempts >= row.MaxAttempts {
		return nil, ErrTooManyAttempts
	}
	if row.CodeHash != hashCode(code) {
		_ = s.q.IncrementOtpAttempts(sctx, row.ID)
		return nil, ErrInvalidCode
	}
	_ = s.q.ConsumeOtpCode(sctx, row.ID)

	userID, isNew, err := s.findOrCreateByIdentity(sctx, tenantID, "phone", phone, func() db.CreateUserParams {
		return db.CreateUserParams{Phone: citextOrNull(phone)}
	})
	if err != nil {
		return nil, err
	}
	if isNew && s.referrer != nil && referralCode != "" {
		s.referrer.AttachSignup(ctx, tenantID, userID, referralCode)
	}
	// A phone login also verifies the identity + sets users.phone if absent.
	_ = s.markIdentityVerified(sctx, userID, "phone", phone)

	return s.issueTokens(ctx, userID, tenantID, uuid.Nil, uuid.Nil, userAgent, ip)
}

// LinkPhone attaches a verified phone to the authenticated user.
func (s *Service) LinkPhone(ctx context.Context, userID uuid.UUID, phoneInput, code, orgCode string) error {
	phone, err := normalizePhone(phoneInput)
	if err != nil {
		return err
	}
	sctx := database.WithSuperAdmin(ctx)
	row, err := s.q.GetLatestOtpCode(sctx, db.GetLatestOtpCodeParams{Destination: phone, Purpose: db.OtpPurposeLogin})
	if err != nil || row.ExpiresAt.Time.Before(time.Now()) || row.CodeHash != hashCode(code) {
		return ErrInvalidCode
	}
	_ = s.q.ConsumeOtpCode(sctx, row.ID)

	if existing, e := s.q.GetAuthIdentity(sctx, db.GetAuthIdentityParams{Provider: "phone", ProviderUid: phone}); e == nil {
		if uuid.UUID(existing.UserID.Bytes) != userID {
			return fmt.Errorf("phone already linked to another account")
		}
		return nil
	}
	_, err = s.q.CreateAuthIdentity(sctx, db.CreateAuthIdentityParams{
		UserID: pgUUID(userID), Provider: "phone", ProviderUid: phone,
		VerifiedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return err
	}
	_, _ = s.q.UpdateUserProfileFields(sctx, db.UpdateUserProfileFieldsParams{ID: pgUUID(userID), Phone: citextOrNull(phone)})
	return nil
}

// findOrCreateByIdentity resolves (provider, providerUID) -> user, creating
// the user + auth_identity + tenant membership on first sight. Returns
// (userID, isNew, error). Runs under a super-admin ctx.
func (s *Service) findOrCreateByIdentity(sctx context.Context, tenantID uuid.UUID, provider, providerUID string, mkUser func() db.CreateUserParams) (uuid.UUID, bool, error) {
	if id, err := s.q.GetAuthIdentity(sctx, db.GetAuthIdentityParams{Provider: db.AuthProvider(provider), ProviderUid: providerUID}); err == nil {
		userID := uuid.UUID(id.UserID.Bytes)
		if err := s.ensureMembership(sctx, tenantID, userID); err != nil {
			return uuid.Nil, false, err
		}
		return userID, false, nil
	}

	var userID uuid.UUID
	err := s.inTx(sctx, func(q *db.Queries) error {
		u, err := q.CreateUser(sctx, mkUser())
		if err != nil {
			return err
		}
		userID = uuid.UUID(u.ID.Bytes)
		if _, err := q.CreateAuthIdentity(sctx, db.CreateAuthIdentityParams{
			UserID: u.ID, Provider: db.AuthProvider(provider), ProviderUid: providerUID,
			VerifiedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		}); err != nil {
			return err
		}
		if _, err := q.AddTenantUser(sctx, db.AddTenantUserParams{
			TenantID: pgUUID(tenantID), UserID: u.ID, Role: db.TenantRoleStudent,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, false, err
	}
	return userID, true, nil
}

func (s *Service) ensureMembership(sctx context.Context, tenantID, userID uuid.UUID) error {
	if _, err := s.q.GetTenantUser(sctx, db.GetTenantUserParams{TenantID: pgUUID(tenantID), UserID: pgUUID(userID)}); err == nil {
		return nil
	}
	_, err := s.q.AddTenantUser(sctx, db.AddTenantUserParams{
		TenantID: pgUUID(tenantID), UserID: pgUUID(userID), Role: db.TenantRoleStudent,
	})
	return err
}

func (s *Service) markIdentityVerified(sctx context.Context, userID uuid.UUID, provider, uid string) error {
	// no-op placeholder — CreateAuthIdentity already sets verified_at on
	// first sight; kept for symmetry with future email-verify flows.
	_ = userID
	_ = provider
	_ = uid
	return nil
}

func citextOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
