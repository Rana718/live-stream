// Package courseorders implements direct course purchase. Lifecycle:
//  1. POST /courses/:id/buy → creates a Razorpay order + a "payments" row
//     with status=created.
//  2. The mobile/web client opens Razorpay checkout with that order_id.
//  3. POST /payments/verify → server verifies the signature, marks the
//     payment paid, creates an enrollment row.
//  4. POST /webhooks/razorpay (idempotent) — backstop for clients that
//     never POST verify.
//
// We pile this on top of the existing payments table rather than introducing
// a new orders table; the schema already had everything we need (provider
// IDs, status, metadata) once we tacked on a course_id.
package courseorders

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"live-platform/internal/database/db"
	"live-platform/internal/events"
	"live-platform/internal/payments"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	rp        *payments.Razorpay
	producer  *events.Producer
	referrals ReferralRewarder
}

func NewService(pool *pgxpool.Pool, rp *payments.Razorpay) *Service {
	return &Service{pool: pool, q: db.New(pool), rp: rp}
}

// WithProducer wires the Kafka producer so successful purchases emit
// payment.succeeded + course.purchased events. Optional.
func (s *Service) WithProducer(p *events.Producer) *Service { s.producer = p; return s }

// ReferralRewarder is the slice of internal/referrals that courseorders
// depends on. Defined here (rather than imported) so the package stays
// import-cycle-free: referrals doesn't know about courseorders, courseorders
// just needs "credit a referrer when this user makes a purchase".
type ReferralRewarder interface {
	RewardOnPurchase(ctx context.Context, referredUser uuid.UUID) (int64, error)
}

// WithReferrals wires the reward-on-first-purchase hook. Optional —
// leaving it nil simply skips the reward step (the purchase still
// completes normally).
func (s *Service) WithReferrals(r ReferralRewarder) *Service { s.referrals = r; return s }

// CreateOrderResult is what the buy endpoint hands back to the client.
// Mirrors what razorpay_flutter / Razorpay JS expects on its checkout call.
type CreateOrderResult struct {
	OrderID   string `json:"order_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	PaymentID string `json:"payment_record_id"` // our internal id
	KeyID     string `json:"key_id,omitempty"`
}

// Buy creates a Razorpay order for the given course + records a pending
// payment row keyed off the order ID.
func (s *Service) Buy(ctx context.Context, tenantID, userID, courseID uuid.UUID, keyID string) (*CreateOrderResult, error) {
	// Already bought? Idempotency for a button-spamming user.
	bought, err := s.q.HasUserBoughtCourse(ctx, db.HasUserBoughtCourseParams{
		UserID:   pgtype.UUID{Bytes: userID, Valid: true},
		CourseID: pgtype.UUID{Bytes: courseID, Valid: true},
	})
	if err == nil && bought {
		return nil, fmt.Errorf("already enrolled")
	}

	course, err := s.q.GetCourseByID(ctx, pgtype.UUID{Bytes: courseID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("course not found")
	}
	priceRupees, _ := course.Price.Float64Value()
	amountPaise := int64(priceRupees.Float64 * 100)
	if amountPaise <= 0 {
		return nil, fmt.Errorf("course is free or unpriced — no order to create")
	}

	receipt := fmt.Sprintf("course-%s-%d", courseID.String()[:8], userID.ID())
	notes := map[string]string{
		"tenant_id": tenantID.String(),
		"user_id":   userID.String(),
		"course_id": courseID.String(),
	}

	// Razorpay Route split. We only attempt the transfer if the tenant has
	// finished Linked-Account KYC AND is on a paid plan. Free tier (starter)
	// keeps everything on the platform account; tenant payouts there happen
	// out-of-band via manual settlement.
	tenant, tErr := s.q.GetTenantByID(ctx, pgtype.UUID{Bytes: tenantID, Valid: true})
	var transfers []payments.Transfer
	if tErr == nil && tenant.RazorpayAccountID.Valid && tenant.RazorpayAccountID.String != "" {
		_, tenantShare := payments.SplitForTenant(amountPaise, tenant.Plan)
		if tenantShare > 0 {
			transfers = []payments.Transfer{{
				Account:  tenant.RazorpayAccountID.String,
				Amount:   tenantShare,
				Currency: "INR",
				Notes:    notes,
			}}
		}
	}

	order, err := s.rp.CreateOrderWithTransfers(ctx, amountPaise, "INR", receipt, notes, transfers)
	if err != nil {
		return nil, err
	}

	meta, _ := json.Marshal(map[string]string{
		"course_title": course.Title,
		"receipt":      receipt,
	})
	row, err := s.q.CreateCourseOrder(ctx, db.CreateCourseOrderParams{
		TenantID:        pgtype.UUID{Bytes: tenantID, Valid: true},
		UserID:          pgtype.UUID{Bytes: userID, Valid: true},
		CourseID:        pgtype.UUID{Bytes: courseID, Valid: true},
		Amount:          course.Price, // rupees, matches payments.amount NUMERIC(10,2)
		Column5:         "INR",
		ProviderOrderID: pgtype.Text{String: order.ID, Valid: true},
		Metadata:        meta,
	})
	if err != nil {
		return nil, err
	}

	return &CreateOrderResult{
		OrderID:   order.ID,
		Amount:    amountPaise,
		Currency:  "INR",
		PaymentID: uuid.UUID(row.ID.Bytes).String(),
		KeyID:     keyID,
	}, nil
}

// VerifyRequest is the payload Razorpay's checkout returns to the client,
// which the client then forwards to /payments/verify.
type VerifyRequest struct {
	RazorpayOrderID   string `json:"razorpay_order_id"`
	RazorpayPaymentID string `json:"razorpay_payment_id"`
	RazorpaySignature string `json:"razorpay_signature"`
}

// Verify validates the signature, marks the payment paid, and enrolls the
// student — atomically. Safe to call multiple times: the row is locked
// FOR UPDATE inside the transaction so concurrent calls serialise, and a
// second call after settlement returns the paid row without re-enrolling
// or re-rewarding.
func (s *Service) Verify(ctx context.Context, req VerifyRequest, userID uuid.UUID) (*db.Payment, error) {
	if !s.rp.VerifyPaymentSignature(req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature) {
		return nil, fmt.Errorf("signature mismatch")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	row, err := qtx.GetCourseOrderByProviderOrderIDForUpdate(ctx,
		pgtype.Text{String: req.RazorpayOrderID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}
	if uuid.UUID(row.UserID.Bytes) != userID {
		return nil, fmt.Errorf("not your order")
	}
	if row.Status.String == "paid" {
		return &row, nil // already settled — idempotent no-op
	}
	if row.Status.String == "refunded" {
		return nil, fmt.Errorf("order was refunded")
	}

	updated, err := qtx.MarkCourseOrderPaid(ctx, db.MarkCourseOrderPaidParams{
		ID:                row.ID,
		ProviderPaymentID: pgtype.Text{String: req.RazorpayPaymentID, Valid: true},
		ProviderSignature: pgtype.Text{String: req.RazorpaySignature, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	// Enrollment is part of the same transaction — a failure here rolls the
	// payment back to unsettled so a retry (client verify or webhook) can
	// complete it. No more "paid but not enrolled" silent gap.
	if _, err := qtx.CreateEnrollment(ctx, db.CreateEnrollmentParams{
		UserID:   row.UserID,
		CourseID: row.CourseID,
		TenantID: row.TenantID,
	}); err != nil {
		return nil, fmt.Errorf("enrollment failed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// --- post-commit, best-effort side effects ---
	tenantID := uuid.UUID(row.TenantID.Bytes)
	courseID := uuid.UUID(row.CourseID.Bytes)
	s.emit(ctx, events.TypePaymentSucceeded, tenantID, userID, map[string]any{
		"order_id":    req.RazorpayOrderID,
		"payment_id":  req.RazorpayPaymentID,
		"course_id":   courseID,
		"amount_paid": row.Amount,
	})
	s.emit(ctx, events.TypeCoursePurchased, tenantID, userID, map[string]any{
		"course_id": courseID,
	})

	if s.referrals != nil {
		if amt, err := s.referrals.RewardOnPurchase(ctx, userID); err == nil && amt > 0 {
			s.emit(ctx, "referral.rewarded", tenantID, userID, map[string]any{
				"reward_paise": amt,
				"course_id":    courseID,
			})
		}
	}

	return &updated, nil
}

// emit publishes an event if a producer is wired; a nil producer is a
// no-op rather than a panic.
func (s *Service) emit(ctx context.Context, t string, tenantID, userID uuid.UUID, payload map[string]any) {
	if s.producer == nil {
		return
	}
	s.producer.Emit(ctx, t, tenantID, userID, payload)
}

// helper: format a uint user ID portion for receipt
//
// Receipt slot in Razorpay is bounded; we keep it short by using the
// trailing 6 chars of the user UUID. This is presentational only — the
// idempotency comes from order_id.
var _ = strconv.Itoa
