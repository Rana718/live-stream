package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"live-platform/internal/database"
	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// GoogleCredential is the client payload for Google sign-in / linking.
type GoogleCredential struct {
	IDToken string `json:"id_token"`
}

func (s *Service) verifyGoogle(ctx context.Context, idToken string) (sub, email, name string, err error) {
	if s.google == nil || !s.google.Enabled() {
		return "", "", "", ErrGoogleDisabled
	}
	if strings.TrimSpace(idToken) == "" {
		return "", "", "", fmt.Errorf("missing google id_token")
	}
	id, err := s.google.Verify(ctx, idToken)
	if err != nil {
		return "", "", "", err
	}
	if !id.EmailVerified {
		return "", "", "", fmt.Errorf("google account email not verified")
	}
	return id.Sub, id.Email, id.FullName, nil
}

// GoogleLogin verifies the ID token, finds-or-creates the user, issues tokens.
func (s *Service) GoogleLogin(ctx context.Context, idToken, orgCode, referralCode, userAgent, ip string) (*TokenBundle, error) {
	sub, email, name, err := s.verifyGoogle(ctx, idToken)
	if err != nil {
		return nil, err
	}
	tenant, err := s.resolveTenant(ctx, orgCode)
	if err != nil {
		return nil, err
	}
	tenantID := uuid.UUID(tenant.ID.Bytes)
	sctx := database.WithSuperAdmin(ctx)

	// If a user already has this verified email, link Google to them.
	if u, e := s.q.GetUserByEmail(sctx, email); e == nil {
		uid := uuid.UUID(u.ID.Bytes)
		if _, e := s.q.GetAuthIdentity(sctx, db.GetAuthIdentityParams{Provider: "google", ProviderUid: sub}); e != nil {
			_, _ = s.q.CreateAuthIdentity(sctx, db.CreateAuthIdentityParams{
				UserID: u.ID, Provider: "google", ProviderUid: sub,
				VerifiedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			})
		}
		if err := s.ensureMembership(sctx, tenantID, uid); err != nil {
			return nil, err
		}
		return s.issueTokens(ctx, uid, tenantID, uuid.Nil, uuid.Nil, userAgent, ip)
	}

	userID, isNew, err := s.findOrCreateByIdentity(sctx, tenantID, "google", sub, func() db.CreateUserParams {
		return db.CreateUserParams{Email: citextOrNull(email), FullName: textOrNull(name)}
	})
	if err != nil {
		return nil, err
	}
	if isNew && s.referrer != nil && referralCode != "" {
		s.referrer.AttachSignup(ctx, tenantID, userID, referralCode)
	}
	return s.issueTokens(ctx, userID, tenantID, uuid.Nil, uuid.Nil, userAgent, ip)
}

// LinkGoogle attaches a verified Google identity to the authenticated user.
func (s *Service) LinkGoogle(ctx context.Context, userID uuid.UUID, idToken string) error {
	sub, _, _, err := s.verifyGoogle(ctx, idToken)
	if err != nil {
		return err
	}
	sctx := database.WithSuperAdmin(ctx)
	if existing, e := s.q.GetAuthIdentity(sctx, db.GetAuthIdentityParams{Provider: "google", ProviderUid: sub}); e == nil {
		if uuid.UUID(existing.UserID.Bytes) != userID {
			return fmt.Errorf("google account already linked elsewhere")
		}
		return nil
	}
	_, err = s.q.CreateAuthIdentity(sctx, db.CreateAuthIdentityParams{
		UserID: pgUUID(userID), Provider: "google", ProviderUid: sub,
		VerifiedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	return err
}
