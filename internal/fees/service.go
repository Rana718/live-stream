package fees

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/payments"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q  *db.Queries
	rp *payments.Razorpay
}

func NewService(pool *pgxpool.Pool, rp *payments.Razorpay) *Service {
	return &Service{q: db.New(pool), rp: rp}
}

// --- Fee structure (admin-defined template per course/batch) ---

type CreateFeeStructureRequest struct {
	CourseID           *uuid.UUID `json:"course_id"`
	BatchID            *uuid.UUID `json:"batch_id"`
	Name               string     `json:"name" validate:"required"`
	TotalAmount        float64    `json:"total_amount" validate:"required,gt=0"`
	Currency           string     `json:"currency"`
	InstallmentsCount  int32      `json:"installments_count"`
	InstallmentGapDays int32      `json:"installment_gap_days"`
}

func (s *Service) CreateStructure(ctx context.Context, req CreateFeeStructureRequest) (*db.FeeStructure, error) {
	if req.Currency == "" {
		req.Currency = "INR"
	}
	if req.InstallmentsCount < 1 {
		req.InstallmentsCount = 1
	}
	if req.InstallmentGapDays < 1 {
		req.InstallmentGapDays = 30
	}
	st, err := s.q.CreateFeeStructure(ctx, db.CreateFeeStructureParams{
		CourseID:           utils.UUIDPtrToPg(req.CourseID),
		BatchID:            utils.UUIDPtrToPg(req.BatchID),
		Name:               req.Name,
		TotalAmount:        utils.NumericFromFloat(req.TotalAmount),
		Currency:           utils.TextToPg(req.Currency),
		InstallmentsCount:  utils.Int4ToPg(req.InstallmentsCount),
		InstallmentGapDays: utils.Int4ToPg(req.InstallmentGapDays),
	})
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Service) ListStructuresByCourse(ctx context.Context, courseID uuid.UUID) ([]db.FeeStructure, error) {
	return s.q.ListFeeStructuresByCourse(ctx, utils.UUIDToPg(courseID))
}

func (s *Service) DeactivateStructure(ctx context.Context, id uuid.UUID) error {
	return s.q.DeactivateFeeStructure(ctx, utils.UUIDToPg(id))
}

// --- Assign fees to a student ---

type AssignFeeRequest struct {
	UserID          uuid.UUID  `json:"user_id" validate:"required"`
	FeeStructureID  *uuid.UUID `json:"fee_structure_id"`
	CourseID        *uuid.UUID `json:"course_id"`
	BatchID         *uuid.UUID `json:"batch_id"`
	TotalAmount     float64    `json:"total_amount" validate:"required,gt=0"`
	Currency        string     `json:"currency"`
	DueDate         *time.Time `json:"due_date"`
	InstallmentsN   int32      `json:"installments_count"`
	InstallmentGap  int32      `json:"installment_gap_days"`
}

// Assign creates a student_fees row + installment rows based on params.
func (s *Service) Assign(ctx context.Context, req AssignFeeRequest) (*db.StudentFee, []db.FeeInstallment, error) {
	if req.Currency == "" {
		req.Currency = "INR"
	}
	if req.InstallmentsN < 1 {
		req.InstallmentsN = 1
	}
	if req.InstallmentGap < 1 {
		req.InstallmentGap = 30
	}

	due := time.Time{}
	if req.DueDate != nil {
		due = *req.DueDate
	}
	sf, err := s.q.CreateStudentFee(ctx, db.CreateStudentFeeParams{
		UserID:         utils.UUIDToPg(req.UserID),
		FeeStructureID: utils.UUIDPtrToPg(req.FeeStructureID),
		CourseID:       utils.UUIDPtrToPg(req.CourseID),
		BatchID:        utils.UUIDPtrToPg(req.BatchID),
		TotalAmount:    utils.NumericFromFloat(req.TotalAmount),
		Currency:       utils.TextToPg(req.Currency),
		DueDate:        utils.DateToPg(due),
	})
	if err != nil {
		return nil, nil, err
	}

	installments := make([]db.FeeInstallment, 0, req.InstallmentsN)
	per := req.TotalAmount / float64(req.InstallmentsN)
	for i := int32(1); i <= req.InstallmentsN; i++ {
		d := time.Time{}
		if !due.IsZero() {
			d = due.AddDate(0, 0, int(i-1)*int(req.InstallmentGap))
		}
		inst, err := s.q.CreateFeeInstallment(ctx, db.CreateFeeInstallmentParams{
			StudentFeeID:      sf.ID,
			InstallmentNumber: i,
			Amount:            utils.NumericFromFloat(per),
			DueDate:           utils.DateToPg(d),
		})
		if err != nil {
			return nil, nil, err
		}
		installments = append(installments, inst)
	}
	return &sf, installments, nil
}

// --- Listings ---

func (s *Service) ListMine(ctx context.Context, userID uuid.UUID) ([]db.ListMyStudentFeesRow, error) {
	return s.q.ListMyStudentFees(ctx, utils.UUIDToPg(userID))
}

func (s *Service) ListPending(ctx context.Context, limit, offset int32) ([]db.ListPendingStudentFeesRow, error) {
	return s.q.ListPendingStudentFees(ctx, db.ListPendingStudentFeesParams{Limit: limit, Offset: offset})
}

func (s *Service) ListOverdueInstallments(ctx context.Context, limit, offset int32) ([]db.ListOverdueInstallmentsRow, error) {
	return s.q.ListOverdueInstallments(ctx, db.ListOverdueInstallmentsParams{Limit: limit, Offset: offset})
}

func (s *Service) GetInstallments(ctx context.Context, studentFeeID uuid.UUID) ([]db.FeeInstallment, error) {
	return s.q.ListInstallmentsForFee(ctx, utils.UUIDToPg(studentFeeID))
}

// --- Pay an installment ---

type PayInstallmentRequest struct {
	InstallmentID uuid.UUID `json:"installment_id" validate:"required"`
}

type PayResponse struct {
	PaymentID     string  `json:"payment_id"`
	RazorpayOrder string  `json:"razorpay_order_id"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	PublicKey     string  `json:"public_key"`
}

// StartInstallmentCheckout creates a Razorpay order for an unpaid installment.
func (s *Service) StartInstallmentCheckout(ctx context.Context, userID uuid.UUID, req PayInstallmentRequest, publicKey string) (*PayResponse, error) {
	if s.rp == nil {
		return nil, errors.New("razorpay not configured")
	}
	installments, err := s.q.ListInstallmentsForFee(ctx, utils.UUIDToPg(uuid.Nil)) // we need direct-by-id lookup
	_ = installments
	if err != nil {
		// not fatal; we'll fetch via a different approach
	}

	// Fetch the installment to get amount + student_fee info
	// Using ListInstallmentsForFee here would require knowing the student_fee_id.
	// We'll use a direct SQL query via sqlc missing — fallback: read student_fees from installment's student_fee_id via a separate query.
	// Simpler: the client sends us the installment_id; we look up from installments by student_fee then filter.
	// For simplicity, we'll accept the client also passing student_fee_id. Let me adjust the request struct instead:
	// Actually the installment_number is unique per student_fee, not global. We need a GetInstallmentByID query.
	// Keep current request contract by reading installments via a dedicated query. For now return a TODO-friendly error.
	return nil, errors.New("not implemented: add GetInstallmentByID query and re-run sqlc")
}

// MarkPaid is invoked after Razorpay verifies a payment; sums up against the parent fee.
func (s *Service) MarkInstallmentPaid(ctx context.Context, installmentID, paymentID uuid.UUID) error {
	inst, err := s.q.MarkInstallmentPaid(ctx, db.MarkInstallmentPaidParams{
		ID:        utils.UUIDToPg(installmentID),
		PaymentID: utils.UUIDToPg(paymentID),
	})
	if err != nil {
		return err
	}
	amount := utils.NumericToFloat(inst.Amount)
	_, err = s.q.UpdateFeePaidAmount(ctx, db.UpdateFeePaidAmountParams{
		ID:         inst.StudentFeeID,
		PaidAmount: utils.NumericFromFloat(amount),
	})
	return err
}

// --- Housekeeping ---

func (s *Service) MarkOverdueFees(ctx context.Context) error {
	return s.q.MarkOverdueFees(ctx)
}

type RevenueSummary struct {
	CapturedTotal float64 `json:"captured_total"`
	PendingTotal  float64 `json:"pending_total"`
	CapturedCount int64   `json:"captured_count"`
	From          string  `json:"from"`
	To            string  `json:"to"`
}

func (s *Service) Revenue(ctx context.Context, from, to time.Time) (*RevenueSummary, error) {
	r, err := s.q.RevenueSummary(ctx, db.RevenueSummaryParams{
		CreatedAt:   utils.TimestampToPg(from),
		CreatedAt_2: utils.TimestampToPg(to),
	})
	if err != nil {
		return nil, err
	}
	return &RevenueSummary{
		CapturedTotal: utils.NumericToFloat(r.CapturedTotal),
		PendingTotal:  utils.NumericToFloat(r.PendingTotal),
		CapturedCount: r.CapturedCount,
		From:          from.Format(time.RFC3339),
		To:            to.Format(time.RFC3339),
	}, nil
}

// Helper to marshal JSON for metadata fields (exported for handlers).
func Marshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// String for compile-time reference to fmt.
var _ = fmt.Sprint
