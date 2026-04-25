// Package devices manages FCM device tokens — one row per device per user.
// The mobile app calls Register on every launch (token can rotate), and the
// notifications service queries TokensForUser before every push.
package devices

import (
	"context"

	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type RegisterInput struct {
	Token    string `json:"token" validate:"required,min=20"`
	Platform string `json:"platform" validate:"required,oneof=android ios web"`
}

// MaxActiveDevices caps how many concurrent devices a single account can
// have registered. Mirrors what PW / Classplus / Unacademy do to prevent
// a single subscription being shared across a coaching centre's worth of
// kids. The cap lives here (not in the DB) so adjusting per-plan is a
// one-line code change.
const MaxActiveDevices = 2

func (s *Service) Register(ctx context.Context, tenantID, userID uuid.UUID, in RegisterInput) error {
	// Evict oldest until we're under the cap. The new token will also count,
	// so the post-condition is `count <= MaxActiveDevices`. We loop instead
	// of computing N-to-evict in one go to be resilient to races.
	for {
		n, err := s.q.CountActiveDevicesForUser(ctx, db.CountActiveDevicesForUserParams{
			TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
			UserID:   pgtype.UUID{Bytes: userID, Valid: true},
		})
		if err != nil || n < int64(MaxActiveDevices) {
			break
		}
		if err := s.q.EvictOldestDeviceForUser(ctx, db.EvictOldestDeviceForUserParams{
			TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
			UserID:   pgtype.UUID{Bytes: userID, Valid: true},
		}); err != nil {
			break
		}
	}
	_, err := s.q.UpsertDeviceToken(ctx, db.UpsertDeviceTokenParams{
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
		UserID:   pgtype.UUID{Bytes: userID, Valid: true},
		Token:    in.Token,
		Platform: in.Platform,
	})
	return err
}

func (s *Service) Unregister(ctx context.Context, token string) error {
	return s.q.DeleteDeviceToken(ctx, token)
}

// TokensForUser returns every active token for a user/tenant. Used by
// notifications.Service to fan out a single push.
func (s *Service) TokensForUser(ctx context.Context, tenantID, userID uuid.UUID) ([]string, error) {
	rows, err := s.q.ListDeviceTokensForUser(ctx, db.ListDeviceTokensForUserParams{
		UserID:   pgtype.UUID{Bytes: userID, Valid: true},
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Token)
	}
	return out, nil
}
