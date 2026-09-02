// Package courseorders implements direct course purchase on the schema-v2
// commerce model: products → prices → orders → order_items → payments →
// entitlements → enrollments. GST is left at zero here — Phase E adds the
// real CGST/SGST/IGST split + invoices.
//
// Lifecycle:
//  1. POST /courses/:id/buy → ensure product+price, create an `orders` row +
//     `order_items` + a Razorpay order + a `payments` row (status=created).
//  2. Client opens Razorpay checkout with the gateway order id.
//  3. POST /payments/verify → verify signature, in one tx: MarkOrderPaid +
//     MarkPaymentCaptured + GrantEntitlement (per item) + UpsertEnrollment.
//  4. Webhook backstop lives in internal/webhooks.
package courseorders

import (
	"context"
	"encoding/json"
	"fmt"

	"live-platform/internal/billing"
	"live-platform/internal/database/db"
	"live-platform/internal/events"
	"live-platform/internal/payments"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	rp        *payments.Razorpay
	producer  *events.Producer
	coupons   CouponRedeemer
	referrals ReferralRewarder
	billing   *billing.Service
}

func NewService(pool *pgxpool.Pool, rp *payments.Razorpay) *Service {
	return &Service{pool: pool, q: db.New(pool), rp: rp, billing: billing.NewService(pool)}
}

func (s *Service) WithProducer(p *events.Producer) *Service { s.producer = p; return s }

type ReferralRewarder interface {
	RewardOnPurchase(ctx context.Context, referredUser uuid.UUID) (int64, error)
}

func (s *Service) WithReferrals(r ReferralRewarder) *Service { s.referrals = r; return s }

// CouponRedeemer is the slice of internal/coupons courseorders needs.
type CouponRedeemer interface {
	Apply(ctx context.Context, tenantID, userID uuid.UUID, code string, amountMinor int, courseID *uuid.UUID, isSubscription bool) (*couponApply, error)
	Redeem(ctx context.Context, tenantID, couponID, userID uuid.UUID, orderID *uuid.UUID, amountOffMinor int) error
}

// couponApply mirrors coupons.ApplyResult without importing it (cycle-free).
type couponApply struct {
	CouponID  uuid.UUID
	Code      string
	AmountOff int
	Final     int
}

func (s *Service) WithCoupons(c CouponRedeemer) *Service { s.coupons = c; return s }

type CreateOrderResult struct {
	OrderID   string `json:"order_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	PaymentID string `json:"payment_record_id"`
	KeyID     string `json:"key_id,omitempty"`
}

type BuyRequest struct {
	CouponCode  string `json:"coupon_code"`
	AmountMinor int64  `json:"amount_minor"` // fallback if the product has no active price
}

// ensureProduct returns the course's product id + kind, creating the product
// row on first sale.
func (s *Service) ensureProduct(ctx context.Context, q *db.Queries, tenantID, courseID uuid.UUID, taxRateBps int32) (pgtype.UUID, error) {
	if p, err := q.GetProductForCourse(ctx, utils.UUIDToPg(courseID)); err == nil {
		return p.ID, nil
	}
	p, err := q.CreateProduct(ctx, db.CreateProductParams{
		TenantID:   utils.UUIDToPg(tenantID),
		Kind:       db.ProductKind("course"),
		CourseID:   utils.UUIDToPg(courseID),
		TaxRateBps: pgtype.Int4{Int32: taxRateBps, Valid: true},
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return p.ID, nil
}

func (s *Service) Buy(ctx context.Context, tenantID, userID, courseID uuid.UUID, keyID string, req BuyRequest) (*CreateOrderResult, error) {
	course, err := s.q.GetCourse(ctx, utils.UUIDToPg(courseID))
	if err != nil {
		return nil, fmt.Errorf("course not found")
	}

	productID, err := s.ensureProduct(ctx, s.q, tenantID, courseID, course.TaxRateBps)
	if err != nil {
		return nil, err
	}

	// Already own it?
	owns, _ := s.q.CheckEntitlement(ctx, db.CheckEntitlementParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), ProductID: productID,
	})
	if owns {
		return nil, fmt.Errorf("already enrolled")
	}

	// Price.
	var amountMinor int64
	if p, e := s.q.GetActivePrice(ctx, productID); e == nil {
		amountMinor = p.AmountMinor
	} else if req.AmountMinor > 0 {
		np, e := s.q.UpsertActivePrice(ctx, db.UpsertActivePriceParams{
			TenantID: utils.UUIDToPg(tenantID), ProductID: productID, AmountMinor: req.AmountMinor,
		})
		if e != nil {
			return nil, e
		}
		amountMinor = np.AmountMinor
	}
	if amountMinor <= 0 {
		return nil, fmt.Errorf("course is free or unpriced — no order to create")
	}

	// Coupon.
	var discount int64
	var couponID pgtype.UUID
	if req.CouponCode != "" && s.coupons != nil {
		ar, e := s.coupons.Apply(ctx, tenantID, userID, req.CouponCode, int(amountMinor), &courseID, false)
		if e != nil {
			return nil, e
		}
		discount = int64(ar.AmountOff)
		couponID = utils.UUIDToPg(ar.CouponID)
	}

	total := amountMinor - discount
	seq, _ := s.q.NextOrderSequence(ctx, utils.UUIDToPg(tenantID))
	code := fmt.Sprintf("ORD-%06d", seq)

	notes, _ := json.Marshal(map[string]string{
		"course_id": courseID.String(), "user_id": userID.String(), "course_title": course.Title,
	})

	rpOrder, err := s.rp.CreateOrder(ctx, total, "INR", code, map[string]string{
		"tenant_id": tenantID.String(), "user_id": userID.String(), "course_id": courseID.String(),
	})
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	order, err := q.CreateOrder(ctx, db.CreateOrderParams{
		TenantID:       utils.UUIDToPg(tenantID),
		UserID:         utils.UUIDToPg(userID),
		Code:           code,
		SubtotalMinor:  amountMinor,
		DiscountMinor:  discount,
		TaxMinor:       0,
		TotalMinor:     total,
		Status:         db.NullOrderStatus{OrderStatus: db.OrderStatus("awaiting_payment"), Valid: true},
		CouponID:       couponID,
		Gateway:        pgtype.Text{String: "razorpay", Valid: true},
		GatewayOrderID: pgtype.Text{String: rpOrder.ID, Valid: true},
		Notes:          notes,
	})
	if err != nil {
		return nil, err
	}

	if _, err := q.CreateOrderItem(ctx, db.CreateOrderItemParams{
		TenantID:          utils.UUIDToPg(tenantID),
		OrderID:           order.ID,
		ProductID:         productID,
		ProductKind:       db.ProductKind("course"),
		Title:             course.Title,
		UnitMinor:         amountMinor,
		Qty:               1,
		LineSubtotalMinor: amountMinor,
		DiscountMinor:     discount,
		TaxableMinor:      total,
		TotalMinor:        total,
		GrantsEntitlement: pgtype.Bool{Bool: true, Valid: true},
	}); err != nil {
		return nil, err
	}

	pay, err := q.CreatePayment(ctx, db.CreatePaymentParams{
		TenantID:       utils.UUIDToPg(tenantID),
		OrderID:        order.ID,
		UserID:         utils.UUIDToPg(userID),
		GatewayOrderID: pgtype.Text{String: rpOrder.ID, Valid: true},
		AmountMinor:    total,
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &CreateOrderResult{
		OrderID: rpOrder.ID, Amount: total, Currency: "INR",
		PaymentID: utils.UUIDFromPg(pay.ID), KeyID: keyID,
	}, nil
}

type VerifyRequest struct {
	RazorpayOrderID   string `json:"razorpay_order_id"`
	RazorpayPaymentID string `json:"razorpay_payment_id"`
	RazorpaySignature string `json:"razorpay_signature"`
}

type VerifyResult struct {
	Status  string `json:"status"`
	OrderID string `json:"order_id"`
}

func (s *Service) Verify(ctx context.Context, req VerifyRequest, userID uuid.UUID) (*VerifyResult, error) {
	if !s.rp.VerifyPaymentSignature(req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature) {
		return nil, fmt.Errorf("signature mismatch")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	order, err := q.GetOrderByGatewayOrderIDForUpdate(ctx, req.RazorpayOrderID)
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}
	if uuid.UUID(order.UserID.Bytes) != userID {
		return nil, fmt.Errorf("not your order")
	}
	if string(order.Status) == "paid" {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &VerifyResult{Status: "paid", OrderID: utils.UUIDFromPg(order.ID)}, nil
	}

	paid, err := q.MarkOrderPaid(ctx, order.ID)
	if err != nil {
		return nil, err
	}

	pays, _ := q.ListPaymentsForOrder(ctx, order.ID)
	for _, p := range pays {
		if string(p.Status) == "created" || string(p.Status) == "authorized" {
			if _, err := q.MarkPaymentCaptured(ctx, db.MarkPaymentCapturedParams{
				ID:               p.ID,
				GatewayPaymentID: pgtype.Text{String: req.RazorpayPaymentID, Valid: true},
				Signature:        pgtype.Text{String: req.RazorpaySignature, Valid: true},
			}); err != nil {
				return nil, err
			}
			break
		}
	}

	items, err := q.ListOrderItems(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	tenantID := uuid.UUID(paid.TenantID.Bytes)
	for _, it := range items {
		if !it.GrantsEntitlement {
			continue
		}
		if _, err := q.GrantEntitlement(ctx, db.GrantEntitlementParams{
			TenantID:    paid.TenantID,
			UserID:      paid.UserID,
			ProductID:   it.ProductID,
			ProductKind: it.ProductKind,
			Source:      db.EntitlementSource("purchase"),
			OrderItemID: it.ID,
		}); err != nil {
			return nil, fmt.Errorf("entitlement grant failed: %w", err)
		}
		if string(it.ProductKind) == "course" {
			if p, e := q.GetProduct(ctx, it.ProductID); e == nil && p.CourseID.Valid {
				if _, err := q.UpsertEnrollment(ctx, db.UpsertEnrollmentParams{
					TenantID: paid.TenantID, UserID: paid.UserID, CourseID: p.CourseID,
				}); err != nil {
					return nil, fmt.Errorf("enrollment failed: %w", err)
				}
			}
		}
	}

	// GST invoice — same tx so numbering stays gapless on the happy path.
	if _, err := s.billing.GenerateForOrder(ctx, q, tenantID, uuid.UUID(order.ID.Bytes)); err != nil {
		return nil, fmt.Errorf("invoice generation failed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// best-effort post-commit
	s.emit(ctx, events.TypeCoursePurchased, tenantID, userID, map[string]any{"order_id": utils.UUIDFromPg(order.ID)})
	if s.referrals != nil {
		_, _ = s.referrals.RewardOnPurchase(ctx, userID)
	}

	return &VerifyResult{Status: "paid", OrderID: utils.UUIDFromPg(order.ID)}, nil
}

func (s *Service) emit(ctx context.Context, t string, tenantID, userID uuid.UUID, payload map[string]any) {
	if s.producer == nil {
		return
	}
	s.producer.Emit(ctx, t, tenantID, userID, payload)
}
