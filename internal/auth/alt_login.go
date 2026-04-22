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

// SendOTP issues a fresh code for `phone`, invalidates any earlier pending
// codes for the same number, and returns the dev code when devModeOTP is on
// (so the app can show it in a debug banner during QA). In production this
// would dispatch via MSG91/Twilio and return only an opaque request ID.
func (s *Service) SendOTP(ctx context.Context, phoneInput string) (phone, devCode string, err error) {
	phone, err = normalizePhone(phoneInput)
	if err != nil {
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

	if devModeOTP {
		// Return the code to the caller so dev clients can skip the SMS leg.
		return phone, code, nil
	}
	return phone, "", nil
}

// VerifyOTP consumes a pending code, returning the matching user (creating one
// if the phone is new). Used for both initial login and linking a phone to an
// already-authenticated account — callers decide how to use the resulting user.
func (s *Service) VerifyOTP(ctx context.Context, phoneInput, code string) (*db.User, error) {
	phone, err := normalizePhone(phoneInput)
	if err != nil {
		return nil, err
	}

	row, err := s.queries.GetLatestSmsCode(ctx, phone)
	if err != nil {
		return nil, fmt.Errorf("no active code for this number")
	}
	if row.Attempts.Int32 >= 5 {
		return nil, fmt.Errorf("too many attempts — request a new code")
	}
	if row.CodeHash != hashCode(code) {
		_ = s.queries.IncrementSmsCodeAttempts(ctx, row.ID)
		return nil, fmt.Errorf("invalid code")
	}
	if err := s.queries.ConsumeSmsCode(ctx, row.ID); err != nil {
		return nil, err
	}

	// Happy path: existing user on this phone?
	if user, err := s.queries.GetUserByPhone(ctx, pgtype.Text{String: phone, Valid: true}); err == nil {
		return &user, nil
	}

	// First time we see this phone — mint a minimal account. The learner
	// will finish their profile via the onboarding flow (class / board / goal).
	email := fmt.Sprintf("%s@mobile.local", strings.TrimPrefix(phone, "+"))
	username := "u" + strings.TrimPrefix(phone, "+")
	user, err := s.queries.CreateUserWithPhone(ctx, db.CreateUserWithPhoneParams{
		Email:       email,
		Username:    username,
		PhoneNumber: pgtype.Text{String: phone, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't create account: %w", err)
	}
	return &user, nil
}

// LoginWithOTP is the handler-facing path: verify + issue tokens.
func (s *Service) LoginWithOTP(ctx context.Context, phone, code string) (*TokenResponse, error) {
	user, err := s.VerifyOTP(ctx, phone, code)
	if err != nil {
		return nil, err
	}
	return s.issueTokensForUser(ctx, user)
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
// and issues tokens. Dev-mode caveat: we trust whatever sub/email the client
// sends. Do not ship to production without adding ID-token verification.
func (s *Service) LoginWithGoogle(ctx context.Context, id GoogleIdentity) (*TokenResponse, error) {
	if id.Sub == "" || id.Email == "" {
		return nil, fmt.Errorf("missing google identity")
	}

	// Prefer google_sub so email rotations don't lose the link.
	if user, err := s.queries.GetUserByGoogleSub(ctx, pgtype.Text{String: id.Sub, Valid: true}); err == nil {
		return s.issueTokensForUser(ctx, &user)
	}

	// Existing account on the same email? Attach google_sub to it.
	if user, err := s.queries.GetUserByEmail(ctx, id.Email); err == nil {
		linked, err := s.queries.LinkGoogleToUser(ctx, db.LinkGoogleToUserParams{
			ID:        user.ID,
			GoogleSub: pgtype.Text{String: id.Sub, Valid: true},
		})
		if err != nil {
			return nil, err
		}
		return s.issueTokensForUser(ctx, &linked)
	}

	// Brand-new account from Google.
	username := "g" + strings.Split(id.Sub, "")[0] + fmt.Sprintf("%d", time.Now().Unix())
	user, err := s.queries.CreateUserWithGoogle(ctx, db.CreateUserWithGoogleParams{
		Email:     id.Email,
		Username:  username,
		FullName:  pgtype.Text{String: id.FullName, Valid: id.FullName != ""},
		GoogleSub: pgtype.Text{String: id.Sub, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return s.issueTokensForUser(ctx, &user)
}

// LinkPhone attaches a verified phone number to an existing authenticated
// account. Separate from LoginWithOTP so the caller can gate it behind an
// access token: only logged-in users can link extra identities.
func (s *Service) LinkPhone(ctx context.Context, userID uuid.UUID, phoneInput, code string) (*db.User, error) {
	phone, err := normalizePhone(phoneInput)
	if err != nil {
		return nil, err
	}
	if _, err := s.VerifyOTP(ctx, phone, code); err != nil {
		return nil, err
	}

	// Phone might already belong to a different user if the learner used OTP
	// login at some point in the past. Refuse rather than silently merge —
	// merging accounts needs explicit UX we haven't built yet.
	if other, err := s.queries.GetUserByPhone(ctx, pgtype.Text{String: phone, Valid: true}); err == nil {
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
func (s *Service) LinkGoogle(ctx context.Context, userID uuid.UUID, id GoogleIdentity) (*db.User, error) {
	if id.Sub == "" {
		return nil, fmt.Errorf("missing google sub")
	}
	if other, err := s.queries.GetUserByGoogleSub(ctx, pgtype.Text{String: id.Sub, Valid: true}); err == nil {
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
