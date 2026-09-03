// Package engagement bundles the lighter "social" features — course reviews,
// forum, gamification (badges + streaks), wishlist, course gifts and course
// chat — into one place. schema-v2 rewrite: everything now goes through sqlc
// (`sql/queries/engagement.sql`) and runs under the caller's RLS scope
// (tenant_id is always in the predicate), replacing the v1 raw-pgx handler
// that referenced dropped columns (`users.role`, `user_streaks`,
// `lecture_notes`).
package engagement

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, q: db.New(pool)} }

func randCode(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func clampLimit(n int32) int32 {
	if n <= 0 || n > 200 {
		return 50
	}
	return n
}

// ─────────────────────────────────────────────────────────────── reviews

func (s *Service) ListReviews(ctx context.Context, tenantID, courseID uuid.UUID, limit, offset int32) ([]db.ListCourseReviewsRow, error) {
	return s.q.ListCourseReviews(ctx, db.ListCourseReviewsParams{
		TenantID: utils.UUIDToPg(tenantID), CourseID: utils.UUIDToPg(courseID),
		Limit: clampLimit(limit), Offset: offset,
	})
}

func (s *Service) ReviewSummary(ctx context.Context, tenantID, courseID uuid.UUID) (db.CourseRatingSummaryRow, error) {
	return s.q.CourseRatingSummary(ctx, db.CourseRatingSummaryParams{
		TenantID: utils.UUIDToPg(tenantID), CourseID: utils.UUIDToPg(courseID),
	})
}

func (s *Service) UpsertReview(ctx context.Context, tenantID, userID, courseID uuid.UUID, rating int, body string) (db.UpsertCourseReviewRow, error) {
	return s.q.UpsertCourseReview(ctx, db.UpsertCourseReviewParams{
		TenantID: utils.UUIDToPg(tenantID), CourseID: utils.UUIDToPg(courseID),
		UserID: utils.UUIDToPg(userID), Rating: int16(rating), Body: utils.TextToPg(body),
	})
}

func (s *Service) AdminListReviews(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.AdminListCourseReviewsRow, error) {
	return s.q.AdminListCourseReviews(ctx, db.AdminListCourseReviewsParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: clampLimit(limit), Offset: offset,
	})
}

func (s *Service) SetReviewApproved(ctx context.Context, reviewID uuid.UUID, approved bool) error {
	return s.q.SetReviewApproved(ctx, db.SetReviewApprovedParams{ID: utils.UUIDToPg(reviewID), IsApproved: approved})
}

func (s *Service) DeleteReview(ctx context.Context, tenantID, reviewID uuid.UUID) error {
	return s.q.DeleteCourseReview(ctx, db.DeleteCourseReviewParams{
		TenantID: utils.UUIDToPg(tenantID), ID: utils.UUIDToPg(reviewID),
	})
}

// ─────────────────────────────────────────────────────────────── forum

func (s *Service) ListThreads(ctx context.Context, tenantID uuid.UUID, courseID *uuid.UUID, limit, offset int32) ([]db.ListForumThreadsRow, error) {
	return s.q.ListForumThreads(ctx, db.ListForumThreadsParams{
		TenantID: utils.UUIDToPg(tenantID), CourseID: utils.UUIDPtrToPg(courseID),
		Limit: clampLimit(limit), Offset: offset,
	})
}

func (s *Service) CreateThread(ctx context.Context, tenantID, userID uuid.UUID, courseID *uuid.UUID, title, body string) (db.CreateForumThreadRow, error) {
	return s.q.CreateForumThread(ctx, db.CreateForumThreadParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
		CourseID: utils.UUIDPtrToPg(courseID), Title: title, Body: utils.TextToPg(body),
	})
}

func (s *Service) GetThread(ctx context.Context, threadID uuid.UUID) (db.GetForumThreadRow, error) {
	return s.q.GetForumThread(ctx, utils.UUIDToPg(threadID))
}

func (s *Service) ListPosts(ctx context.Context, threadID uuid.UUID, limit, offset int32) ([]db.ListForumPostsRow, error) {
	return s.q.ListForumPosts(ctx, db.ListForumPostsParams{
		ThreadID: utils.UUIDToPg(threadID), Limit: clampLimit(limit), Offset: offset,
	})
}

// AddPost inserts a reply and bumps the thread's counters in one transaction.
// Instructors/admins get their posts flagged as highlighted.
func (s *Service) AddPost(ctx context.Context, tenantID, threadID, userID uuid.UUID, body string, highlight bool) (db.AddForumPostRow, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.AddForumPostRow{}, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	post, err := qtx.AddForumPost(ctx, db.AddForumPostParams{
		TenantID: utils.UUIDToPg(tenantID), ThreadID: utils.UUIDToPg(threadID),
		UserID: utils.UUIDToPg(userID), Body: body,
	})
	if err != nil {
		return db.AddForumPostRow{}, err
	}
	if err := qtx.BumpForumThread(ctx, utils.UUIDToPg(threadID)); err != nil {
		return db.AddForumPostRow{}, err
	}
	if highlight {
		if err := qtx.HighlightForumPost(ctx, db.HighlightForumPostParams{ID: post.ID, IsInstructorHighlight: true}); err != nil {
			return db.AddForumPostRow{}, err
		}
		post.IsInstructorHighlight = true
	}
	if err := tx.Commit(ctx); err != nil {
		return db.AddForumPostRow{}, err
	}
	return post, nil
}

func (s *Service) SetThreadPinned(ctx context.Context, tenantID, threadID uuid.UUID, pinned bool) error {
	return s.q.SetForumThreadPinned(ctx, db.SetForumThreadPinnedParams{
		TenantID: utils.UUIDToPg(tenantID), ID: utils.UUIDToPg(threadID), IsPinned: pinned,
	})
}

func (s *Service) SetThreadLocked(ctx context.Context, tenantID, threadID uuid.UUID, locked bool) error {
	return s.q.SetForumThreadLocked(ctx, db.SetForumThreadLockedParams{
		TenantID: utils.UUIDToPg(tenantID), ID: utils.UUIDToPg(threadID), IsLocked: locked,
	})
}

// ─────────────────────────────────────────────────────────── gamification

func (s *Service) ListBadges(ctx context.Context) ([]db.ListBadgesRow, error) {
	return s.q.ListBadges(ctx)
}

type Gamification struct {
	Streak db.GetLearningStreakRow
	Badges []db.ListUserBadgesRow
}

func (s *Service) MyGamification(ctx context.Context, tenantID, userID uuid.UUID) (Gamification, error) {
	var g Gamification
	streak, err := s.q.GetLearningStreak(ctx, db.GetLearningStreakParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return g, err
	}
	g.Streak = streak
	g.Badges, err = s.q.ListUserBadges(ctx, db.ListUserBadgesParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
	return g, err
}

// CheckIn extends the daily streak (idempotent per calendar day) and
// auto-grants streak badges so they light up immediately.
func (s *Service) CheckIn(ctx context.Context, tenantID, userID uuid.UUID) (Gamification, error) {
	row, err := s.q.UpsertLearningStreak(ctx, db.UpsertLearningStreakParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
	if err != nil {
		return Gamification{}, err
	}
	// Grant streak badges by code when the threshold is reached.
	thresholds := map[string]int32{"streak_7": 7, "streak_30": 30}
	badges, _ := s.q.ListBadges(ctx)
	for _, b := range badges {
		if need, ok := thresholds[b.Code]; ok && row.CurrentStreak >= need {
			_ = s.q.GrantBadge(ctx, db.GrantBadgeParams{
				TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), BadgeID: b.ID,
			})
		}
	}
	return s.MyGamification(ctx, tenantID, userID)
}

func (s *Service) Leaderboard(ctx context.Context, tenantID uuid.UUID, limit int32) ([]db.LeaderboardByPointsRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.q.LeaderboardByPoints(ctx, db.LeaderboardByPointsParams{TenantID: utils.UUIDToPg(tenantID), Limit: limit})
}

// ─────────────────────────────────────────────────────────────── wishlist

func (s *Service) ListWishlist(ctx context.Context, tenantID, userID uuid.UUID) ([]db.ListWishlistRow, error) {
	return s.q.ListWishlist(ctx, db.ListWishlistParams{TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID)})
}

func (s *Service) AddWishlist(ctx context.Context, tenantID, userID, courseID uuid.UUID) error {
	return s.q.AddToWishlist(ctx, db.AddToWishlistParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), CourseID: utils.UUIDToPg(courseID),
	})
}

func (s *Service) RemoveWishlist(ctx context.Context, tenantID, userID, courseID uuid.UUID) error {
	return s.q.RemoveFromWishlist(ctx, db.RemoveFromWishlistParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), CourseID: utils.UUIDToPg(courseID),
	})
}

// ─────────────────────────────────────────────────────────────── gifts

type CreateGiftInput struct {
	ProductID      *uuid.UUID
	OrderID        *uuid.UUID
	RecipientPhone string
	RecipientEmail string
	Message        string
}

func (s *Service) CreateGift(ctx context.Context, tenantID, senderID uuid.UUID, in CreateGiftInput) (db.CreateCourseGiftRow, error) {
	return s.q.CreateCourseGift(ctx, db.CreateCourseGiftParams{
		TenantID: utils.UUIDToPg(tenantID), SenderID: utils.UUIDToPg(senderID),
		RedemptionCode: "GIFT-" + randCode(4),
		OrderID:        utils.UUIDPtrToPg(in.OrderID), ProductID: utils.UUIDPtrToPg(in.ProductID),
		RecipientPhone: utils.TextToPg(in.RecipientPhone), RecipientEmail: utils.TextToPg(in.RecipientEmail),
		Message: utils.TextToPg(in.Message),
	})
}

func (s *Service) ListMyGifts(ctx context.Context, tenantID, senderID uuid.UUID) ([]db.ListMyCourseGiftsRow, error) {
	return s.q.ListMyCourseGifts(ctx, db.ListMyCourseGiftsParams{
		TenantID: utils.UUIDToPg(tenantID), SenderID: utils.UUIDToPg(senderID),
	})
}

// RedeemGift marks a gift code consumed. Entitlement fan-out from the gift's
// product is handled by the commerce/entitlements path when the redeemer
// buys/enrols — here we only settle the gift record.
func (s *Service) RedeemGift(ctx context.Context, redeemerID uuid.UUID, code string) (db.RedeemCourseGiftRow, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	row, err := s.q.RedeemCourseGift(ctx, db.RedeemCourseGiftParams{
		RedemptionCode: code, RedeemedBy: utils.UUIDToPg(redeemerID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.RedeemCourseGiftRow{}, ErrGiftNotRedeemable
	}
	return row, err
}

var ErrGiftNotRedeemable = errors.New("invalid or already-redeemed gift code")

// ─────────────────────────────────────────────────────────── course chat

func (s *Service) ListCourseChat(ctx context.Context, tenantID, courseID uuid.UUID, limit, offset int32) ([]db.ListCourseChatRow, error) {
	return s.q.ListCourseChat(ctx, db.ListCourseChatParams{
		TenantID: utils.UUIDToPg(tenantID), CourseID: utils.UUIDToPg(courseID),
		Limit: clampLimit(limit), Offset: offset,
	})
}

func (s *Service) SendCourseChat(ctx context.Context, tenantID, courseID, userID uuid.UUID, body string) (db.PostCourseChatRow, error) {
	return s.q.PostCourseChat(ctx, db.PostCourseChatParams{
		TenantID: utils.UUIDToPg(tenantID), CourseID: utils.UUIDToPg(courseID),
		UserID: utils.UUIDToPg(userID), Body: body,
	})
}
