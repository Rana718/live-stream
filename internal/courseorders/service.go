// Package courseorders implements direct course purchase. Lifecycle:
//   1. POST /courses/:id/buy → creates a Razorpay order + a "payments" row
//      with status=created.
//   2. The mobile/web client opens Razorpay checkout with that order_id.
//   3. POST /payments/verify → server verifies the signature, marks the
//      payment paid, creates an enrollment row.
//   4. POST /webhooks/razorpay (idempotent) — backstop for clients that
//      never POST verify.
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
	q        *db.Queries
	rp       *payments.Razorpay
	producer *events.Producer
}

func NewService(pool *pgxpool.Pool, rp *payments.Razorpay) *Service {
	return &Service{q: db.New(pool), rp: rp}
}

// WithProducer wires the Kafka producer so successful purchases emit
// payment.succeeded + course.purchased events. Optional.
func (s *Service) WithProducer(p *events.Producer) *Service { s.producer = p; return s }

// CreateOrderResult is what the buy endpoint hands back to the client.
// Mirrors what razorpay_flutter / Razorpay JS expects on its checkout call.
type CreateOrderResult struct {
	OrderID    string `json:"order_id"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	PaymentID  string `json:"payment_record_id"` // our internal id
	KeyID      string `json:"key_id,omitempty"`
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
		Amount:          pgtype.Numeric{Int: nil, Exp: 0, Valid: true},
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
// student. Safe to call multiple times — second call no-ops.
func (s *Service) Verify(ctx context.Context, req VerifyRequest, userID uuid.UUID) (*db.Payment, error) {
	if !s.rp.VerifyPaymentSignature(req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature) {
		return nil, fmt.Errorf("signature mismatch")
	}

	row, err := s.q.GetCourseOrderByProviderOrderID(ctx, pgtype.Text{String: req.RazorpayOrderID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}
	if uuid.UUID(row.UserID.Bytes) != userID {
		return nil, fmt.Errorf("not your order")
	}
	if row.Status.String == "paid" {
		return &row, nil // idempotent
	}

	updated, err := s.q.MarkCourseOrderPaid(ctx, db.MarkCourseOrderPaidParams{
		ID:                row.ID,
		ProviderPaymentID: pgtype.Text{String: req.RazorpayPaymentID, Valid: true},
		ProviderSignature: pgtype.Text{String: req.RazorpaySignature, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	// Auto-enroll the student. CreateEnrollment is generated by sqlc; we
	// pass the course_id straight from the payment row.
	_, _ = s.q.CreateEnrollment(ctx, db.CreateEnrollmentParams{
		UserID:   row.UserID,
		CourseID: row.CourseID,
		TenantID: row.TenantID,
	})

	// Fire-and-forget event publication. Downstream consumers (push
	// notifications, analytics roll-ups, audit) react to these without
	// adding latency to the verify response.
	tenantID := uuid.UUID(row.TenantID.Bytes)
	courseID := uuid.UUID(row.CourseID.Bytes)
	s.producer.Emit(ctx, events.TypePaymentSucceeded, tenantID, userID, map[string]any{
		"order_id":    req.RazorpayOrderID,
		"payment_id":  req.RazorpayPaymentID,
		"course_id":   courseID,
		"amount_paid": row.Amount,
	})
	s.producer.Emit(ctx, events.TypeCoursePurchased, tenantID, userID, map[string]any{
		"course_id": courseID,
	})

	return &updated, nil
}

// helper: format a uint user ID portion for receipt
//
// Receipt slot in Razorpay is bounded; we keep it short by using the
// trailing 6 chars of the user UUID. This is presentational only — the
// idempotency comes from order_id.
var _ = strconv.Itoa
