// Package refunds — schema-v2. Real `refunds` table (payment_id RESTRICT,
// reason enum, status pending→processed). A full refund revokes the order's
// entitlements and flips the order to refunded.
package refunds

import (
	"context"
	"fmt"

	"live-platform/internal/billing"
	"live-platform/internal/database/db"
	"live-platform/internal/email"
	"live-platform/internal/payments"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q       *db.Queries
	rp      *payments.Razorpay
	email   email.Client
	billing *billing.Service
}

func NewService(pool *pgxpool.Pool, rp *payments.Razorpay) *Service {
	return &Service{q: db.New(pool), rp: rp, billing: billing.NewService(pool)}
}

func (s *Service) WithEmail(c email.Client) *Service { s.email = c; return s }

type IssueInput struct {
	PaymentID   string `json:"payment_id" validate:"required,uuid"`
	AmountPaise int64  `json:"amount_paise"`
	AmountMinor int64  `json:"amount_minor"`
	Reason      string `json:"reason" validate:"required,min=4,max=500"`
	Speed       string `json:"speed"`
}

func (in IssueInput) amount() int64 {
	if in.AmountMinor > 0 {
		return in.AmountMinor
	}
	return in.AmountPaise
}

type IssueResult struct {
	RefundID        string `json:"refund_id"`
	GatewayRefundID string `json:"gateway_refund_id"`
	AmountMinor     int64  `json:"amount_minor"`
	Status          string `json:"status"`
	EmailSent       bool   `json:"email_sent"`
}

func reasonEnum(s string) db.NullRefundReason {
	switch s {
	case "duplicate", "fraud", "goodwill", "chargeback":
		return db.NullRefundReason{RefundReason: db.RefundReason(s), Valid: true}
	default:
		return db.NullRefundReason{RefundReason: db.RefundReason("requested_by_customer"), Valid: true}
	}
}

func (s *Service) Issue(ctx context.Context, tenantID, adminID uuid.UUID, in IssueInput) (*IssueResult, error) {
	pid, err := uuid.Parse(in.PaymentID)
	if err != nil {
		return nil, fmt.Errorf("invalid payment_id")
	}
	pay, err := s.q.GetPaymentByIDForTenant(ctx, db.GetPaymentByIDForTenantParams{
		ID: utils.UUIDToPg(pid), TenantID: utils.UUIDToPg(tenantID),
	})
	if err != nil {
		return nil, fmt.Errorf("payment not found in this tenant")
	}
	if string(pay.Status) != "captured" {
		return nil, fmt.Errorf("payment is %q — only captured payments can be refunded", pay.Status)
	}
	if !pay.GatewayPaymentID.Valid || pay.GatewayPaymentID.String == "" {
		return nil, fmt.Errorf("payment has no gateway payment id — cannot refund automatically")
	}

	amount := in.amount()
	full := amount == 0 || amount >= pay.AmountMinor
	if full {
		already, _ := s.q.SumRefundedForPayment(ctx, pay.ID)
		amount = pay.AmountMinor - already
	}
	if amount <= 0 {
		return nil, fmt.Errorf("nothing left to refund")
	}

	item, _ := s.q.FirstGrantsEntitlementOrderItem(ctx, pay.OrderID)

	ref, err := s.q.CreateRefund(ctx, db.CreateRefundParams{
		TenantID:    utils.UUIDToPg(tenantID),
		PaymentID:   pay.ID,
		AmountMinor: amount,
		OrderItemID: item.ID,
		Reason:      reasonEnum(in.Reason),
		InitiatedBy: utils.UUIDToPg(adminID),
	})
	if err != nil {
		return nil, err
	}

	rzp, err := s.rp.CreateRefund(ctx, pay.GatewayPaymentID.String, amount, in.Speed,
		utils.UUIDFromPg(ref.ID), map[string]string{"reason": in.Reason, "tenant_id": tenantID.String()})
	if err != nil {
		_ = s.q.MarkRefundFailed(ctx, db.MarkRefundFailedParams{ID: ref.ID, Notes: pgtype.Text{String: err.Error(), Valid: true}})
		return nil, fmt.Errorf("razorpay refund failed: %w", err)
	}

	done, err := s.q.MarkRefundProcessed(ctx, db.MarkRefundProcessedParams{
		ID: ref.ID, GatewayRefundID: pgtype.Text{String: rzp.ID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("razorpay refunded (%s) but DB patch failed: %w", rzp.ID, err)
	}

	if full && item.ID.Valid {
		_ = s.q.RevokeEntitlementsForOrderItem(ctx, db.RevokeEntitlementsForOrderItemParams{
			OrderItemID: item.ID, RevokeReason: pgtype.Text{String: "refunded", Valid: true},
		})
		_ = s.q.CancelEnrollmentsForOrderItem(ctx, item.ID)
		_ = s.q.SetOrderStatus(ctx, db.SetOrderStatusParams{ID: pay.OrderID, Status: db.OrderStatus("refunded")})
	} else {
		_ = s.q.SetOrderStatus(ctx, db.SetOrderStatusParams{ID: pay.OrderID, Status: db.OrderStatus("partially_refunded")})
	}

	// GST credit note reversing the refunded slice.
	if _, err := s.billing.GenerateForRefund(ctx, s.q, tenantID,
		uuid.UUID(pay.OrderID.Bytes), uuid.UUID(done.ID.Bytes), amount, in.Reason); err != nil {
		// non-fatal: the money moved; a missing credit note is a reconciliation task
		_ = err
	}

	emailSent := false
	if s.email != nil {
		if u, e := s.q.GetUserByID(ctx, pay.UserID); e == nil && u.Email.Valid && u.Email.String != "" {
			name := "there"
			if u.FullName.Valid {
				name = u.FullName.String
			}
			err := s.email.SendTemplate(ctx, u.Email.String, "refund_issued", map[string]any{
				"UserName": name, "AmountRupees": fmt.Sprintf("%.2f", float64(amount)/100), "Reason": in.Reason,
			})
			emailSent = err == nil
		}
	}

	return &IssueResult{
		RefundID: utils.UUIDFromPg(done.ID), GatewayRefundID: rzp.ID,
		AmountMinor: amount, Status: string(done.Status), EmailSent: emailSent,
	}, nil
}
