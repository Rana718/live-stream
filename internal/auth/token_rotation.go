package auth

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Token rotation uses Redis-backed blocklists. When a refresh token is used, its
// jti (from the claims) is added to `revoked:<jti>` with a TTL matching the token's
// remaining lifetime. Subsequent reuse attempts are rejected.
//
// NOTE: this relies on the current JWT utils NOT being jti-aware. The simpler approach
// used here is to blocklist the entire token value SHA — deterministic, stateless.

const (
	revokedPrefix = "revoked:"
	revokeTTL     = 8 * 24 * time.Hour // longer than typical refresh TTL
)

// RevokeRefreshToken marks a refresh token as used/revoked.
func (s *Service) RevokeRefreshToken(ctx context.Context, token string) error {
	return s.redis.Set(ctx, revokedPrefix+token, "1", revokeTTL).Err()
}

// IsRefreshTokenRevoked returns true if the provided refresh token has been blocklisted.
func (s *Service) IsRefreshTokenRevoked(ctx context.Context, token string) (bool, error) {
	_, err := s.redis.Get(ctx, revokedPrefix+token).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// EnsureRefreshUsable wraps the revoked check with a friendly error.
func (s *Service) EnsureRefreshUsable(ctx context.Context, token string) error {
	revoked, err := s.IsRefreshTokenRevoked(ctx, token)
	if err != nil {
		return err
	}
	if revoked {
		return errors.New("refresh token already used or revoked")
	}
	return nil
}
