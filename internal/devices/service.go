// Package devices manages push tokens — one row per device per user, capped
// per account to deter subscription sharing.
package devices

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

// MaxActiveDevices caps concurrent registered devices per account.
const MaxActiveDevices = 3

type RegisterInput struct {
	Token    string `json:"token" validate:"required,min=20"`
	Platform string `json:"platform" validate:"required,oneof=android ios web"`
}

func (s *Service) Register(ctx context.Context, tenantID, userID uuid.UUID, in RegisterInput) error {
	if err := s.q.RegisterDeviceToken(ctx, db.RegisterDeviceTokenParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
		Token: in.Token, Platform: in.Platform,
	}); err != nil {
		return err
	}
	return s.q.TrimOldestDeviceTokens(ctx, db.TrimOldestDeviceTokensParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
		Keep: MaxActiveDevices,
	})
}

func (s *Service) Unregister(ctx context.Context, token string) error {
	return s.q.RevokeDeviceToken(ctx, token)
}

// TokensForUser returns active push tokens for fan-out.
func (s *Service) TokensForUser(ctx context.Context, tenantID, userID uuid.UUID) ([]string, error) {
	rows, err := s.q.ListActiveDeviceTokens(ctx, db.ListActiveDeviceTokensParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
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
