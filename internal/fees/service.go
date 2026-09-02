// Package fees — schema-v2. fee_structures→fee_plans, student_fees→
// fee_accounts (total/paid/waived _minor), fee_installments carry a seq and
// an optional order_id. Installment split: first N-1 = floor(total/N), last
// = remainder. Each installment payment is a one-time order.
package fees

import (
	"context"
	"errors"
	"fmt"
	"time"

	"live-platform/internal/billing"
	"live-platform/internal/database/db"
	"live-platform/internal/payments"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	rp      *payments.Razorpay
	billing *billing.Service
}

func NewService(pool *pgxpool.Pool, rp *payments.Razorpay) *Service {
	return &Service{pool: pool, q: db.New(pool), rp: rp, billing: billing.NewService(pool)}
}

func dptr(t *time.Time) pgtype.Date {
	if t == nil || t.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// ── fee plans ───────────────────────────────────────────────────────

type CreateFeeStructureRequest struct {
	CourseID           *uuid.UUID `json:"course_id"`
	BatchID            *uuid.UUID `json:"batch_id"`
	Name               string     `json:"name" validate:"required"`
	TotalAmount        float64    `json:"total_amount" validate:"required,gt=0"`
	TotalMinor         int64      `json:"total_minor"`
	InstallmentsCount  int32      `json:"installments_count"`
	InstallmentGapDays int32      `json:"installment_gap_days"`
	LateFeeMinor       int64      `json:"late_fee_minor"`
}

func (r CreateFeeStructureRequest) total() int64 {
	if r.TotalMinor > 0 {
		return r.TotalMinor
	}
	return int64(r.TotalAmount * 100)
}

func (s *Service) CreateStructure(ctx context.Context, tenantID uuid.UUID, req CreateFeeStructureRequest) (db.CreateFeePlanRow, error) {
	n := req.InstallmentsCount
	if n < 1 {
		n = 1
	}
	gap := req.InstallmentGapDays
	if gap < 1 {
		gap = 30
	}
	return s.q.CreateFeePlan(ctx, db.CreateFeePlanParams{
		TenantID:          utils.UUIDToPg(tenantID),
		Name:              req.Name,
		TotalMinor:        req.total(),
		CourseID:          utils.UUIDPtrToPg(req.CourseID),
		BatchID:           utils.UUIDPtrToPg(req.BatchID),
		InstallmentsCount: pgtype.Int4{Int32: n, Valid: true},
		GapDays:           pgtype.Int4{Int32: gap, Valid: true},
		LateFeeMinor:      pgtype.Int8{Int64: req.LateFeeMinor, Valid: req.LateFeeMinor > 0},
	})
}

func (s *Service) ListStructuresByCourse(ctx context.Context, tenantID, courseID uuid.UUID) ([]db.ListFeePlansByCourseRow, error) {
	return s.q.ListFeePlansByCourse(ctx, db.ListFeePlansByCourseParams{
		TenantID: utils.UUIDToPg(tenantID), CourseID: utils.UUIDToPg(courseID),
	})
}

func (s *Service) ListStructuresForTenant(ctx context.Context, tenantID uuid.UUID) ([]db.ListFeePlansForTenantRow, error) {
	return s.q.ListFeePlansForTenant(ctx, utils.UUIDToPg(tenantID))
}

// ── assign ──────────────────────────────────────────────────────────

type AssignFeeRequest struct {
	UserID         uuid.UUID  `json:"user_id" validate:"required"`
	FeeStructureID *uuid.UUID `json:"fee_structure_id"`
	FeePlanID      *uuid.UUID `json:"fee_plan_id"`
	CourseID       *uuid.UUID `json:"course_id"`
	BatchID        *uuid.UUID `json:"batch_id"`
	TotalAmount    float64    `json:"total_amount" validate:"required,gt=0"`
	TotalMinor     int64      `json:"total_minor"`
	Installments   int32      `json:"installments"`
	GapDays        int32      `json:"gap_days"`
	FirstDueDate   *time.Time `json:"first_due_date"`
}

func (r AssignFeeRequest) total() int64 {
	if r.TotalMinor > 0 {
		return r.TotalMinor
	}
	return int64(r.TotalAmount * 100)
}

func (s *Service) Assign(ctx context.Context, tenantID uuid.UUID, req AssignFeeRequest) (db.CreateFeeAccountRow, []db.CreateFeeInstallmentRow, error) {
	n := req.Installments
	if n < 1 {
		n = 1
	}
	gap := req.GapDays
	if gap < 1 {
		gap = 30
	}
	total := req.total()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.CreateFeeAccountRow{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	planID := req.FeePlanID
	if planID == nil {
		planID = req.FeeStructureID
	}
	acc, err := q.CreateFeeAccount(ctx, db.CreateFeeAccountParams{
		TenantID:   utils.UUIDToPg(tenantID),
		UserID:     utils.UUIDToPg(req.UserID),
		TotalMinor: total,
		FeePlanID:  utils.UUIDPtrToPg(planID),
		CourseID:   utils.UUIDPtrToPg(req.CourseID),
		BatchID:    utils.UUIDPtrToPg(req.BatchID),
		DueOn:      dptr(req.FirstDueDate),
	})
	if err != nil {
		return db.CreateFeeAccountRow{}, nil, err
	}

	base := total / int64(n)
	due := time.Now()
	if req.FirstDueDate != nil {
		due = *req.FirstDueDate
	}
	var insts []db.CreateFeeInstallmentRow
	for i := int32(1); i <= n; i++ {
		amt := base
		if i == n {
			amt = total - base*int64(n-1)
		}
		d := due.AddDate(0, 0, int(gap)*int(i-1))
		row, e := q.CreateFeeInstallment(ctx, db.CreateFeeInstallmentParams{
			TenantID: utils.UUIDToPg(tenantID), FeeAccountID: acc.ID, Seq: i,
			AmountMinor: amt, DueOn: pgtype.Date{Time: d, Valid: true},
		})
		if e != nil {
			return db.CreateFeeAccountRow{}, nil, e
		}
		insts = append(insts, row)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.CreateFeeAccountRow{}, nil, err
	}
	return acc, insts, nil
}

// ── student / admin lists ───────────────────────────────────────────

func (s *Service) ListMine(ctx context.Context, tenantID, userID uuid.UUID) ([]db.ListFeeInstallmentsForUserRow, error) {
	return s.q.ListFeeInstallmentsForUser(ctx, db.ListFeeInstallmentsForUserParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
}

func (s *Service) ListPending(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.ListPendingFeeAccountsRow, error) {
	return s.q.ListPendingFeeAccounts(ctx, db.ListPendingFeeAccountsParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
}

func (s *Service) ListOverdueInstallments(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.ListOverdueFeeInstallmentsRow, error) {
	return s.q.ListOverdueFeeInstallments(ctx, db.ListOverdueFeeInstallmentsParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
}

func (s *Service) GetInstallments(ctx context.Context, feeAccountID uuid.UUID) ([]db.ListFeeInstallmentsRow, error) {
	return s.q.ListFeeInstallments(ctx, utils.UUIDToPg(feeAccountID))
}

// ── installment payment ─────────────────────────────────────────────

type PayInstallmentRequest struct {
	InstallmentID uuid.UUID `json:"installment_id" validate:"required"`
}

type PayResponse struct {
	InstallmentID string `json:"installment_id"`
	RazorpayOrder string `json:"razorpay_order_id"`
	OrderID       string `json:"order_id"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	PublicKey     string `json:"public_key"`
}

func (s *Service) StartInstallmentCheckout(ctx context.Context, tenantID, userID uuid.UUID, req PayInstallmentRequest, publicKey string) (*PayResponse, error) {
	inst, err := s.q.GetFeeInstallment(ctx, utils.UUIDToPg(req.InstallmentID))
	if err != nil {
		return nil, fmt.Errorf("installment not found")
	}
	if string(inst.Status) == "paid" {
		return nil, fmt.Errorf("already paid")
	}
	if s.rp == nil {
		return nil, errors.New("razorpay not configured")
	}
	total := inst.AmountMinor
	seq, _ := s.q.NextOrderSequence(ctx, utils.UUIDToPg(tenantID))
	code := fmt.Sprintf("ORD-%06d", seq)
	rpOrder, err := s.rp.CreateOrder(ctx, total, "INR", code, map[string]string{
		"tenant_id": tenantID.String(), "user_id": userID.String(),
		"installment_id": req.InstallmentID.String(),
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
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), Code: code,
		SubtotalMinor: total, TotalMinor: total,
		Status:         db.NullOrderStatus{OrderStatus: db.OrderStatus("awaiting_payment"), Valid: true},
		Gateway:        pgtype.Text{String: "razorpay", Valid: true},
		GatewayOrderID: pgtype.Text{String: rpOrder.ID, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	if _, err := q.CreatePayment(ctx, db.CreatePaymentParams{
		TenantID: utils.UUIDToPg(tenantID), OrderID: order.ID, UserID: utils.UUIDToPg(userID),
		GatewayOrderID: pgtype.Text{String: rpOrder.ID, Valid: true}, AmountMinor: total,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &PayResponse{
		InstallmentID: req.InstallmentID.String(), RazorpayOrder: rpOrder.ID, OrderID: rpOrder.ID,
		Amount: total, Currency: "INR", PublicKey: publicKey,
	}, nil
}

type VerifyInstallmentRequest struct {
	InstallmentID     uuid.UUID `json:"installment_id" validate:"required"`
	RazorpayOrderID   string    `json:"razorpay_order_id" validate:"required"`
	RazorpayPaymentID string    `json:"razorpay_payment_id" validate:"required"`
	RazorpaySignature string    `json:"razorpay_signature" validate:"required"`
}

func (s *Service) VerifyInstallmentPayment(ctx context.Context, userID uuid.UUID, req VerifyInstallmentRequest) error {
	if s.rp == nil || !s.rp.VerifyPaymentSignature(req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature) {
		return errors.New("invalid signature")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	order, err := q.GetOrderByGatewayOrderIDForUpdate(ctx, req.RazorpayOrderID)
	if err != nil {
		return fmt.Errorf("order not found")
	}
	if uuid.UUID(order.UserID.Bytes) != userID {
		return errors.New("forbidden")
	}
	if string(order.Status) != "paid" {
		if _, err := q.MarkOrderPaid(ctx, order.ID); err != nil {
			return err
		}
		pays, _ := q.ListPaymentsForOrder(ctx, order.ID)
		for _, p := range pays {
			if string(p.Status) == "created" || string(p.Status) == "authorized" {
				_, _ = q.MarkPaymentCaptured(ctx, db.MarkPaymentCapturedParams{
					ID: p.ID, GatewayPaymentID: pgtype.Text{String: req.RazorpayPaymentID, Valid: true},
					Signature: pgtype.Text{String: req.RazorpaySignature, Valid: true},
				})
				break
			}
		}
	}

	inst, err := q.MarkFeeInstallmentPaid(ctx, db.MarkFeeInstallmentPaidParams{
		ID: utils.UUIDToPg(req.InstallmentID), OrderID: order.ID,
	})
	if err != nil {
		return err
	}
	if _, err := q.AddFeeAccountPayment(ctx, db.AddFeeAccountPaymentParams{
		ID: inst.FeeAccountID, PaidMinor: inst.AmountMinor,
	}); err != nil {
		return err
	}
	if _, err := s.billing.GenerateForOrder(ctx, q, uuid.UUID(order.TenantID.Bytes), uuid.UUID(order.ID.Bytes)); err != nil {
		return fmt.Errorf("invoice generation failed: %w", err)
	}
	return tx.Commit(ctx)
}

type RevenueSummary struct {
	CapturedTotal float64 `json:"captured_total"`
	PendingTotal  float64 `json:"pending_total"`
	CapturedCount int64   `json:"captured_count"`
}

func (s *Service) Revenue(ctx context.Context, from, to time.Time) (*RevenueSummary, error) {
	// Fee revenue is folded into the unified orders/payments analytics in
	// Phase G; return zeros here.
	return &RevenueSummary{}, nil
}
