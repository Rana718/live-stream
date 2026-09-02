// Package referrals — schema-v2. Per-user referral_codes; referral_events
// (pending → rewarded) with reward_minor. RewardOnPurchase bumps the
// referred user's pending event once, on first purchase.
package referrals

import (
	"context"
	"crypto/rand"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultRewardMinor — ₹100 in paise.
const DefaultRewardMinor = 10000

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

func generateCode() (string, error) {
	const alpha = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alpha[int(buf[i])%len(alpha)]
	}
	return string(buf), nil
}

type MyCodeResult struct {
	Code               string `json:"code"`
	Uses               int    `json:"uses"`
	PendingCount       int64  `json:"pending_count"`
	RewardedCount      int64  `json:"rewarded_count"`
	TotalRewardedPaise int64  `json:"total_rewarded_paise"`
}

func (s *Service) MyCode(ctx context.Context, tenantID, userID uuid.UUID) (*MyCodeResult, error) {
	code, err := generateCode()
	if err != nil {
		return nil, err
	}
	row, err := s.q.GetOrCreateReferralCode(ctx, db.GetOrCreateReferralCodeParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), Code: code,
	})
	if err != nil {
		return nil, err
	}
	stats, err := s.q.ReferralStatsForUser(ctx, db.ReferralStatsForUserParams{
		TenantID: utils.UUIDToPg(tenantID), ReferrerUserID: utils.UUIDToPg(userID),
	})
	if err != nil {
		stats = db.ReferralStatsForUserRow{}
	}
	return &MyCodeResult{
		Code:               row.Code,
		Uses:               int(row.Uses),
		PendingCount:       stats.Total - stats.Rewarded,
		RewardedCount:      stats.Rewarded,
		TotalRewardedPaise: stats.TotalRewardMinor,
	}, nil
}

// AttachToSignup — best-effort, called from the OTP verify path.
func (s *Service) AttachToSignup(ctx context.Context, tenantID, newUserID uuid.UUID, code string) {
	if code == "" {
		return
	}
	rc, err := s.q.GetReferralCodeByCode(ctx, code)
	if err != nil {
		return
	}
	if uuid.UUID(rc.UserID.Bytes) == newUserID || uuid.UUID(rc.TenantID.Bytes) != tenantID {
		return
	}
	_, _ = s.q.CreateReferralEvent(ctx, db.CreateReferralEventParams{
		TenantID:       utils.UUIDToPg(tenantID),
		Code:           code,
		ReferrerUserID: rc.UserID,
		ReferredUserID: utils.UUIDToPg(newUserID),
	})
	_ = s.q.IncrementReferralCodeUses(ctx, rc.ID)
}

// RewardOnPurchase bumps the referred user's pending event to rewarded.
// Idempotent — no pending event → returns (0, nil).
func (s *Service) RewardOnPurchase(ctx context.Context, referredUser uuid.UUID) (int64, error) {
	ev, err := s.q.GetPendingReferralForUser(ctx, utils.UUIDToPg(referredUser))
	if err != nil {
		return 0, nil // no pending event
	}
	if err := s.q.MarkReferralRewarded(ctx, db.MarkReferralRewardedParams{
		ID:          ev.ID,
		RewardMinor: DefaultRewardMinor,
	}); err != nil {
		return 0, err
	}
	return DefaultRewardMinor, nil
}
