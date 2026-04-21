package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// Password-reset and email-verification tokens are stored in Redis with short TTLs.
// No email provider is wired — the token is returned in the response in non-production envs.
// For production, hook into SendGrid/Postmark/etc. in DispatchPasswordResetEmail / DispatchVerificationEmail.

const (
	resetTokenTTL   = 30 * time.Minute
	verifyTokenTTL  = 24 * time.Hour
	resetKeyPrefix  = "pwreset:"
	verifyKeyPrefix = "verify:"
)

func genToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// StartPasswordReset generates a one-time reset token keyed in Redis.
// Return value is the opaque token that would normally be emailed to the user.
// In development envs this is returned to the caller for convenience.
func (s *Service) StartPasswordReset(ctx context.Context, email string) (string, error) {
	u, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		// Don't leak which emails exist — return a token anyway.
		return genToken(24)
	}
	token, err := genToken(24)
	if err != nil {
		return "", err
	}
	key := resetKeyPrefix + token
	if err := s.redis.Set(ctx, key, utils.UUIDFromPg(u.ID), resetTokenTTL).Err(); err != nil {
		return "", err
	}
	return token, nil
}

type CompletePasswordResetRequest struct {
	Token       string `json:"token" validate:"required,min=16"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

func (s *Service) CompletePasswordReset(ctx context.Context, req CompletePasswordResetRequest) error {
	key := resetKeyPrefix + req.Token
	uid, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return errors.New("invalid or expired reset token")
	}
	if err != nil {
		return err
	}
	parsedID, err := uuid.Parse(uid)
	if err != nil {
		return fmt.Errorf("corrupt token payload")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := s.queries.AdminResetUserPassword(ctx, db.AdminResetUserPasswordParams{
		ID:           utils.UUIDToPg(parsedID),
		PasswordHash: string(hash),
	}); err != nil {
		return err
	}
	_ = s.redis.Del(ctx, key).Err()
	return nil
}

// StartEmailVerification generates a verification token. In production this
// would be sent to the user's email; here we return it to the caller so the
// frontend can display or log it in dev.
func (s *Service) StartEmailVerification(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := genToken(24)
	if err != nil {
		return "", err
	}
	key := verifyKeyPrefix + token
	if err := s.redis.Set(ctx, key, userID.String(), verifyTokenTTL).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) CompleteEmailVerification(ctx context.Context, token string) error {
	key := verifyKeyPrefix + token
	uid, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return errors.New("invalid or expired verification token")
	}
	if err != nil {
		return err
	}
	parsedID, err := uuid.Parse(uid)
	if err != nil {
		return fmt.Errorf("corrupt token payload")
	}
	if _, err := s.queries.VerifyUserEmail(ctx, utils.UUIDToPg(parsedID)); err != nil {
		return err
	}
	_ = s.redis.Del(ctx, key).Err()
	return nil
}
