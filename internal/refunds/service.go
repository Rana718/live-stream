// Package refunds gives tenant admins a one-call refund flow:
//   1. Validate the payment row belongs to the caller's tenant.
//   2. Hit Razorpay's refund API (idempotent on payment_id).
//   3. Patch the payments row to status=refunded with the metadata block.
//   4. Send the user an email receipt of the refund.
//
// Reverse cases we do NOT handle here:
//   - Subscription refunds (separate Razorpay flow, lives in subscriptions).
//   - Bank-side bounces or chargeback disputes (handled via webhook).
//
// Why we expose this to tenant admins (not super_admin only): Indian
// e-commerce regulation requires the seller of record to honour refunds,
// and the tenant is that seller. Platform staff would be the bottleneck.
package refunds

import (
	"context"
	"fmt"

	"live-platform/internal/database/db"
	"live-platform/internal/email"
	"live-platform/internal/payments"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q     *db.Queries
	rp    *payments.Razorpay
	email email.Client // nil = email disabled, refund still works
}

func NewService(pool *pgxpool.Pool, rp *payments.Razorpay) *Service {
	return &Service{q: db.New(pool), rp: rp}
}

func (s *Service) WithEmail(c email.Client) *Service { s.email = c; return s }

// IssueInput is the admin-form payload. All fields are validated server-
// side; we don't trust the client-side form to compute the partial-refund
// amount — Razorpay does that authoritatively from `amount_paise`.
type IssueInput struct {
	PaymentID string `json:"payment_id" validate:"required,uuid"`
	// AmountPaise = 0 means "full refund".
	AmountPaise int64  `json:"amount_paise"`
	Reason      string `json:"reason" validate:"required,min=4,max=500"`
	// Speed: "normal" (T+2-3 days, free) or "optimum" (instant where
	// possible, slightly higher MDR). Default normal.
	Speed string `json:"speed"`
}

// IssueResult tells the admin UI both our internal payment-row state +
// the Razorpay refund record. The latter is what the user sees in their
// bank app.
type IssueResult struct {
	Refund      *payments.Refund `json:"razorpay_refund"`
	PaymentRow  *db.Payment      `json:"payment"`
	EmailSent   bool             `json:"email_sent"`
}

// Issue runs the four-step flow above. Returns IssueResult so the admin
// UI can render "₹500 refunded — Razorpay refund ID rfnd_XXX".
func (s *Service) Issue(ctx context.Context, tenantID uuid.UUID, in IssueInput) (*IssueResult, error) {
	pid, err := uuid.Parse(in.PaymentID)
	if err != nil {
		return nil, fmt.Errorf("invalid payment_id")
	}

	// 1. Validate ownership (RLS would block too, but explicit is clearer).
	row, err := s.q.GetPaymentByIDForTenant(ctx, db.GetPaymentByIDForTenantParams{
		ID:       utils.UUIDToPg(pid),
		TenantID: utils.UUIDToPg(tenantID),
	})
	if err != nil {
		return nil, fmt.Errorf("payment not found in this tenant")
	}
	if row.Status.String != "paid" {
		return nil, fmt.Errorf("payment is %q — only paid orders can be refunded", row.Status.String)
	}
	if !row.ProviderPaymentID.Valid || row.ProviderPaymentID.String == "" {
		return nil, fmt.Errorf("payment has no razorpay payment id — cannot refund automatically")
	}

	// 2. Hit Razorpay. Idempotency key = our internal payment UUID, so
	//    a double-click in the admin UI doesn't double-refund.
	rzpRefund, err := s.rp.CreateRefund(
		ctx,
		row.ProviderPaymentID.String,
		in.AmountPaise,
		in.Speed,
		uuid.UUID(row.ID.Bytes).String(),
		map[string]string{
			"reason":    in.Reason,
			"tenant_id": tenantID.String(),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("razorpay refund failed: %w", err)
	}

	// 3. Patch the payment row.
	updated, err := s.q.MarkPaymentRefunded(ctx, db.MarkPaymentRefundedParams{
		ID:      row.ID,
		Column2: rzpRefund.ID,
		Column3: rzpRefund.Amount,
		Column4: in.Reason,
	})
	if err != nil {
		// Razorpay-side refund already initiated — surface the inconsistency
		// to the caller but keep the refund-id in the error so support can
		// reconcile manually.
		return nil, fmt.Errorf("razorpay refunded (%s) but DB patch failed: %w", rzpRefund.ID, err)
	}

	// 4. Email the user. Best-effort — a missing email is fine, an SMTP
	//    failure shouldn't undo the refund.
	emailSent := false
	if s.email != nil {
		// Pull the user + tenant rows so the receipt has names/amounts.
		user, uerr := s.q.GetUserByID(ctx, row.UserID)
		if uerr == nil && user.Email.Valid && user.Email.String != "" {
			tenant, terr := s.q.GetTenantByID(ctx, row.TenantID)
			tenantName := "School"
			if terr == nil {
				tenantName = tenant.Name
			}
			rupees := float64(rzpRefund.Amount) / 100.0
			emailErr := s.email.SendTemplate(ctx, user.Email.String,
				"refund_issued", map[string]any{
					"UserName":     userDisplayName(&user),
					"TenantName":   tenantName,
					"AmountRupees": fmt.Sprintf("%.2f", rupees),
					"Reason":       in.Reason,
					"OrderID":      derefText(row.ProviderOrderID),
				})
			emailSent = emailErr == nil
		}
	}

	return &IssueResult{
		Refund:     rzpRefund,
		PaymentRow: &updated,
		EmailSent:  emailSent,
	}, nil
}

// userDisplayName picks the most-human-readable name for the receipt.
// Falls back to the phone if no full name was set.
func userDisplayName(u *db.User) string {
	if u.FullName.Valid && u.FullName.String != "" {
		return u.FullName.String
	}
	if u.PhoneNumber.Valid {
		return u.PhoneNumber.String
	}
	return "there"
}

func derefText(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}
