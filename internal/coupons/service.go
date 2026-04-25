// Package coupons issues + validates discount codes a tenant can apply to
// course purchases, fee payments, and subscription checkouts.
//
// Validation rules (in order, all must pass):
//   1. Coupon exists, belongs to the tenant, is_active = true.
//   2. now() between starts_at and ends_at.
//   3. amount >= min_amount.
//   4. usage_limit not yet reached (used_count < limit).
//   5. scope == 'all' OR (scope == 'course' AND coupon_courses includes the
//      course) OR (scope == 'subscription' AND a plan_id is supplied).
//   6. The user hasn't redeemed THIS coupon before (per-user-once policy).
//
// Discount calculation: percent caps at max_discount when set.
package coupons

import (
	"context"
	"fmt"
	"strings"
	"time"

	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type CreateInput struct {
	Code          string     `json:"code" validate:"required,min=4,max=40"`
	DiscountType  string     `json:"discount_type" validate:"required,oneof=percent flat"`
	DiscountValue int        `json:"discount_value" validate:"required,min=1"`
	MaxDiscount   *int       `json:"max_discount"`
	Scope         string     `json:"scope" validate:"required,oneof=all course subscription"`
	MinAmount     int        `json:"min_amount"`
	StartsAt      *time.Time `json:"starts_at"`
	EndsAt        *time.Time `json:"ends_at"`
	UsageLimit    *int       `json:"usage_limit"`
	CourseIDs     []uuid.UUID `json:"course_ids"`
}

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, in CreateInput) (*db.Coupon, error) {
	starts := time.Now().UTC()
	if in.StartsAt != nil {
		starts = *in.StartsAt
	}
	var ends pgtype.Timestamptz
	if in.EndsAt != nil {
		ends = pgtype.Timestamptz{Time: *in.EndsAt, Valid: true}
	}
	maxDisc := pgtype.Int4{}
	if in.MaxDiscount != nil {
		maxDisc = pgtype.Int4{Int32: int32(*in.MaxDiscount), Valid: true}
	}
	usage := pgtype.Int4{}
	if in.UsageLimit != nil {
		usage = pgtype.Int4{Int32: int32(*in.UsageLimit), Valid: true}
	}

	coupon, err := s.q.CreateCoupon(ctx, db.CreateCouponParams{
		TenantID:      pgtype.UUID{Bytes: tenantID, Valid: true},
		Code:          in.Code,
		DiscountType:  in.DiscountType,
		DiscountValue: int32(in.DiscountValue),
		MaxDiscount:   maxDisc,
		Scope:         in.Scope,
		MinAmount:     int32(in.MinAmount),
		StartsAt:      pgtype.Timestamptz{Time: starts, Valid: true},
		EndsAt:        ends,
		UsageLimit:    usage,
	})
	if err != nil {
		return nil, err
	}
	for _, cid := range in.CourseIDs {
		_ = s.q.AttachCouponToCourse(ctx, db.AttachCouponToCourseParams{
			CouponID: coupon.ID,
			CourseID: pgtype.UUID{Bytes: cid, Valid: true},
		})
	}
	return &coupon, nil
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.Coupon, error) {
	return s.q.ListCoupons(ctx, db.ListCouponsParams{
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
		Limit:    limit,
		Offset:   offset,
	})
}

func (s *Service) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	return s.q.SetCouponActive(ctx, db.SetCouponActiveParams{
		ID:       pgtype.UUID{Bytes: id, Valid: true},
		IsActive: active,
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteCoupon(ctx, pgtype.UUID{Bytes: id, Valid: true})
}

// ApplyResult is returned by Apply: the caller (course purchase / fees / etc.)
// charges (amount - amount_off) to Razorpay.
type ApplyResult struct {
	CouponID  uuid.UUID `json:"coupon_id"`
	Code      string    `json:"code"`
	AmountOff int       `json:"amount_off"`     // paise
	Final     int       `json:"final_amount"`   // paise after discount
	Message   string    `json:"message,omitempty"`
}

// Apply validates a code against an in-progress purchase. It does not
// mark the coupon as redeemed — call Redeem after the payment is verified
// so a failed checkout doesn't burn a single-use coupon.
func (s *Service) Apply(ctx context.Context, tenantID, userID uuid.UUID, code string,
	amountPaise int, courseID *uuid.UUID, isSubscription bool) (*ApplyResult, error) {

	code = strings.ToUpper(strings.TrimSpace(code))
	coupon, err := s.q.GetCouponByCode(ctx, db.GetCouponByCodeParams{
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
		Upper:    code,
	})
	if err != nil {
		return nil, fmt.Errorf("invalid coupon")
	}

	now := time.Now().UTC()
	if !coupon.IsActive || coupon.StartsAt.Time.After(now) {
		return nil, fmt.Errorf("coupon not active yet")
	}
	if coupon.EndsAt.Valid && coupon.EndsAt.Time.Before(now) {
		return nil, fmt.Errorf("coupon expired")
	}
	if amountPaise < int(coupon.MinAmount) {
		return nil, fmt.Errorf("minimum amount not met")
	}
	if coupon.UsageLimit.Valid && coupon.UsedCount >= coupon.UsageLimit.Int32 {
		return nil, fmt.Errorf("coupon exhausted")
	}

	switch coupon.Scope {
	case "course":
		if courseID == nil {
			return nil, fmt.Errorf("coupon only applies on course purchase")
		}
		ids, _ := s.q.ListCouponCourses(ctx, coupon.ID)
		ok := false
		for _, id := range ids {
			if uuid.UUID(id.Bytes) == *courseID {
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("coupon doesn't apply to this course")
		}
	case "subscription":
		if !isSubscription {
			return nil, fmt.Errorf("coupon only applies on subscription checkout")
		}
	}

	// Per-user-once policy.
	prior, _ := s.q.CountCouponRedemptionsByUser(ctx, db.CountCouponRedemptionsByUserParams{
		CouponID: coupon.ID,
		UserID:   pgtype.UUID{Bytes: userID, Valid: true},
	})
	if prior > 0 {
		return nil, fmt.Errorf("already redeemed by this user")
	}

	// Compute discount.
	off := 0
	switch coupon.DiscountType {
	case "percent":
		off = amountPaise * int(coupon.DiscountValue) / 100
		if coupon.MaxDiscount.Valid && off > int(coupon.MaxDiscount.Int32) {
			off = int(coupon.MaxDiscount.Int32)
		}
	case "flat":
		off = int(coupon.DiscountValue)
	}
	if off > amountPaise {
		off = amountPaise
	}
	return &ApplyResult{
		CouponID:  uuid.UUID(coupon.ID.Bytes),
		Code:      coupon.Code,
		AmountOff: off,
		Final:     amountPaise - off,
	}, nil
}

// Redeem records a successful redemption and increments usage. Call from
// the payment-verify handler after the charge succeeds. amount_off should
// match what Apply returned for this same checkout.
func (s *Service) Redeem(ctx context.Context, tenantID, couponID, userID uuid.UUID,
	paymentID *uuid.UUID, amountOff int) error {

	pay := pgtype.UUID{}
	if paymentID != nil {
		pay = pgtype.UUID{Bytes: *paymentID, Valid: true}
	}
	_, err := s.q.RecordCouponRedemption(ctx, db.RecordCouponRedemptionParams{
		TenantID:  pgtype.UUID{Bytes: tenantID, Valid: true},
		CouponID:  pgtype.UUID{Bytes: couponID, Valid: true},
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		PaymentID: pay,
		AmountOff: int32(amountOff),
	})
	if err != nil {
		return err
	}
	return s.q.IncrementCouponUsage(ctx, pgtype.UUID{Bytes: couponID, Valid: true})
}
