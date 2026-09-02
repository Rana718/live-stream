// Package coupons — schema-v2. coupons.type (percent|flat), percent_bps /
// value_minor / max_discount_minor / min_order_minor, per_user_limit,
// applies_to (all|products|categories). Product scoping via coupon_products.
package coupons

import (
	"context"
	"fmt"
	"strings"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

func ntext(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

type CreateInput struct {
	Code          string      `json:"code" validate:"required,min=4,max=40"`
	DiscountType  string      `json:"discount_type" validate:"required,oneof=percent flat"`
	DiscountValue int         `json:"discount_value" validate:"required,min=1"`
	MaxDiscount   *int        `json:"max_discount"` // paise
	Scope         string      `json:"scope"`
	MinAmount     int         `json:"min_amount"` // paise
	StartsAt      *time.Time  `json:"starts_at"`
	EndsAt        *time.Time  `json:"ends_at"`
	UsageLimit    *int        `json:"usage_limit"`
	PerUserLimit  *int        `json:"per_user_limit"`
	ProductIDs    []uuid.UUID `json:"product_ids"`
	CourseIDs     []uuid.UUID `json:"course_ids"` // resolved to products
}

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, in CreateInput) (db.CreateCouponRow, error) {
	scope := in.Scope
	if scope == "" || scope == "course" || scope == "subscription" {
		scope = "all"
	}
	if len(in.ProductIDs) > 0 || len(in.CourseIDs) > 0 {
		scope = "products"
	}

	var percentBps, valueMinor pgtype.Int8
	pbps := pgtype.Int4{}
	if in.DiscountType == "percent" {
		pbps = pgtype.Int4{Int32: int32(in.DiscountValue) * 100, Valid: true}
	} else {
		valueMinor = pgtype.Int8{Int64: int64(in.DiscountValue), Valid: true}
	}
	_ = percentBps

	maxDisc := pgtype.Int8{}
	if in.MaxDiscount != nil {
		maxDisc = pgtype.Int8{Int64: int64(*in.MaxDiscount), Valid: true}
	}
	usage := pgtype.Int4{}
	if in.UsageLimit != nil {
		usage = pgtype.Int4{Int32: int32(*in.UsageLimit), Valid: true}
	}
	perUser := pgtype.Int4{Int32: 1, Valid: true}
	if in.PerUserLimit != nil {
		perUser = pgtype.Int4{Int32: int32(*in.PerUserLimit), Valid: true}
	}
	var ends pgtype.Timestamptz
	if in.EndsAt != nil {
		ends = pgtype.Timestamptz{Time: *in.EndsAt, Valid: true}
	}
	starts := pgtype.Timestamptz{}
	if in.StartsAt != nil {
		starts = pgtype.Timestamptz{Time: *in.StartsAt, Valid: true}
	}

	row, err := s.q.CreateCoupon(ctx, db.CreateCouponParams{
		TenantID:         utils.UUIDToPg(tenantID),
		Code:             strings.ToUpper(strings.TrimSpace(in.Code)),
		Type:             db.CouponType(in.DiscountType),
		PercentBps:       pbps,
		ValueMinor:       valueMinor,
		MaxDiscountMinor: maxDisc,
		MinOrderMinor:    pgtype.Int8{Int64: int64(in.MinAmount), Valid: in.MinAmount > 0},
		AppliesTo:        db.NullCouponScope{CouponScope: db.CouponScope(scope), Valid: true},
		StartsAt:         starts,
		EndsAt:           ends,
		UsageLimit:       usage,
		PerUserLimit:     perUser,
	})
	if err != nil {
		return db.CreateCouponRow{}, err
	}
	for _, pid := range in.ProductIDs {
		_ = s.q.AttachCouponToProduct(ctx, db.AttachCouponToProductParams{
			TenantID: utils.UUIDToPg(tenantID), CouponID: row.ID, ProductID: utils.UUIDToPg(pid),
		})
	}
	for _, cid := range in.CourseIDs {
		if p, e := s.q.GetProductForCourse(ctx, utils.UUIDToPg(cid)); e == nil {
			_ = s.q.AttachCouponToProduct(ctx, db.AttachCouponToProductParams{
				TenantID: utils.UUIDToPg(tenantID), CouponID: row.ID, ProductID: p.ID,
			})
		}
	}
	return row, nil
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.ListCouponsRow, error) {
	return s.q.ListCoupons(ctx, db.ListCouponsParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
}

func (s *Service) SetActive(ctx context.Context, tenantID, id uuid.UUID, active bool) error {
	return s.q.SetCouponActive(ctx, db.SetCouponActiveParams{
		ID: utils.UUIDToPg(id), TenantID: utils.UUIDToPg(tenantID), IsActive: active,
	})
}

func (s *Service) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.q.DeleteCoupon(ctx, db.DeleteCouponParams{
		ID: utils.UUIDToPg(id), TenantID: utils.UUIDToPg(tenantID),
	})
}

// ApplyResult — the caller charges (final_amount) to the gateway.
type ApplyResult struct {
	CouponID  uuid.UUID `json:"coupon_id"`
	Code      string    `json:"code"`
	AmountOff int       `json:"amount_off"`
	Final     int       `json:"final_amount"`
}

func (s *Service) Apply(ctx context.Context, tenantID, userID uuid.UUID, code string,
	amountMinor int, courseID *uuid.UUID, isSubscription bool) (*ApplyResult, error) {

	code = strings.ToUpper(strings.TrimSpace(code))
	cp, err := s.q.GetCouponByCode(ctx, db.GetCouponByCodeParams{
		TenantID: utils.UUIDToPg(tenantID), Code: code,
	})
	if err != nil {
		return nil, fmt.Errorf("invalid coupon")
	}

	now := time.Now().UTC()
	if !cp.IsActive || cp.StartsAt.Time.After(now) {
		return nil, fmt.Errorf("coupon not active yet")
	}
	if cp.EndsAt.Valid && cp.EndsAt.Time.Before(now) {
		return nil, fmt.Errorf("coupon expired")
	}
	if int64(amountMinor) < cp.MinOrderMinor {
		return nil, fmt.Errorf("minimum amount not met")
	}
	if cp.UsageLimit.Valid && cp.UsedCount >= cp.UsageLimit.Int32 {
		return nil, fmt.Errorf("coupon exhausted")
	}

	if string(cp.AppliesTo) == "products" && courseID != nil {
		if p, e := s.q.GetProductForCourse(ctx, utils.UUIDToPg(*courseID)); e == nil {
			ok, _ := s.q.CouponAllowsProduct(ctx, db.CouponAllowsProductParams{
				CouponID: cp.ID, ProductID: p.ID,
			})
			if !ok.Bool {
				return nil, fmt.Errorf("coupon doesn't apply to this course")
			}
		}
	}

	prior, _ := s.q.CountUserCouponRedemptions(ctx, db.CountUserCouponRedemptionsParams{
		CouponID: cp.ID, UserID: utils.UUIDToPg(userID),
	})
	if int(prior) >= int(cp.PerUserLimit) {
		return nil, fmt.Errorf("redemption limit reached for this user")
	}

	off := 0
	switch string(cp.Type) {
	case "percent":
		off = amountMinor * int(cp.PercentBps.Int32) / 10000
		if cp.MaxDiscountMinor.Valid && int64(off) > cp.MaxDiscountMinor.Int64 {
			off = int(cp.MaxDiscountMinor.Int64)
		}
	case "flat":
		off = int(cp.ValueMinor.Int64)
	}
	if off > amountMinor {
		off = amountMinor
	}
	return &ApplyResult{
		CouponID: uuid.UUID(cp.ID.Bytes), Code: cp.Code, AmountOff: off, Final: amountMinor - off,
	}, nil
}

// Redeem records a redemption + bumps usage. Call after payment success.
func (s *Service) Redeem(ctx context.Context, tenantID, couponID, userID uuid.UUID, orderID *uuid.UUID, amountOffMinor int) error {
	_, err := s.q.RecordCouponRedemption(ctx, db.RecordCouponRedemptionParams{
		TenantID:       utils.UUIDToPg(tenantID),
		CouponID:       utils.UUIDToPg(couponID),
		UserID:         utils.UUIDToPg(userID),
		OrderID:        utils.UUIDPtrToPg(orderID),
		AmountOffMinor: int64(amountOffMinor),
	})
	if err != nil {
		return err
	}
	return s.q.IncrementCouponUsage(ctx, utils.UUIDToPg(couponID))
}
