// Package coursebundles implements admin-curated multi-course combos. The
// student-facing replacement for the subscription model: instead of paying
// monthly for everything, students buy individual courses, and bundles
// stack two or three at a discount.
//
// Buy / verify mirrors the single-course flow in `courseorders` — same
// Razorpay order shape, same payments table, same Route splits — but
// fan-out enrolls the student into every course in the bundle on verify.
package coursebundles

import (
	"context"
	"encoding/json"
	"fmt"

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

func (s *Service) WithProducer(p *events.Producer) *Service { s.producer = p; return s }

// BundleView is the student-store payload. Includes a "save_paise" field
// computed server-side so we don't repeat the math on every client.
type BundleView struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	PricePaise       int32    `json:"price_paise"`
	MemberPricePaise int32    `json:"member_price_paise"`
	SavePaise        int32    `json:"save_paise"`
	CourseIDs        []string `json:"course_ids"`
	CoverURL         string   `json:"cover_url"`
}

// List returns all active bundles for the current tenant. RLS in Postgres
// scopes by tenant — the Go side just relays.
func (s *Service) List(ctx context.Context) ([]BundleView, error) {
	rows, err := s.q.ListCourseBundles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BundleView, 0, len(rows))
	for _, r := range rows {
		// course_ids comes back as `interface{}` because it's a Postgres
		// uuid[] aggregate; pgx decodes it as []any of [16]byte. Normalise
		// to the string form clients expect.
		ids := decodeUUIDArray(r.CourseIds)
		member := r.MemberPricePaise
		save := member - r.PricePaise
		if save < 0 {
			// Bundle priced higher than members? Don't show a negative
			// savings number; admin probably misconfigured.
			save = 0
		}
		out = append(out, BundleView{
			ID:               uuid.UUID(r.ID.Bytes).String(),
			Title:            r.Title,
			Description:      r.Description.String,
			PricePaise:       r.PricePaise,
			MemberPricePaise: member,
			SavePaise:        save,
			CourseIDs:        ids,
			CoverURL:         r.CoverUrl.String,
		})
	}
	return out, nil
}

// Buy creates a Razorpay order for a bundle. Returns the same shape as
// the single-course Buy so the client checkout code can be reused.
func (s *Service) Buy(ctx context.Context, tenantID, userID, bundleID uuid.UUID, keyID string) (*BuyResult, error) {
	bundle, err := s.q.GetCourseBundleByID(ctx, pgtype.UUID{Bytes: bundleID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("bundle not found")
	}
	if !bundle.IsActive {
		return nil, fmt.Errorf("bundle not for sale")
	}
	amountPaise := int64(bundle.PricePaise)
	if amountPaise <= 0 {
		return nil, fmt.Errorf("bundle has no price set")
	}

	receipt := fmt.Sprintf("bundle-%s", bundleID.String()[:8])
	notes := map[string]string{
		"tenant_id": tenantID.String(),
		"user_id":   userID.String(),
		"bundle_id": bundleID.String(),
	}

	// Same Route-split logic as single-course purchases. We don't compute
	// a different split for bundles — the platform commission rate keys
	// off the tenant's plan, not the SKU type.
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

	// We piggy-back on course_orders for the payment row. course_id is
	// nullable on that table; bundle purchases set it NULL and stash the
	// bundle_id in metadata. (verify reads metadata.bundle_id to fan out.)
	meta, _ := json.Marshal(map[string]string{
		"bundle_id":    bundleID.String(),
		"bundle_title": bundle.Title,
		"receipt":      receipt,
	})
	row, err := s.q.CreateBundleOrder(ctx, db.CreateBundleOrderParams{
		TenantID:        pgtype.UUID{Bytes: tenantID, Valid: true},
		UserID:          pgtype.UUID{Bytes: userID, Valid: true},
		Amount:          pgtype.Numeric{Int: nil, Exp: 0, Valid: true},
		Column4:         "INR",
		ProviderOrderID: pgtype.Text{String: order.ID, Valid: true},
		Metadata:        meta,
	})
	if err != nil {
		return nil, err
	}

	return &BuyResult{
		OrderID:   order.ID,
		Amount:    amountPaise,
		Currency:  "INR",
		PaymentID: uuid.UUID(row.ID.Bytes).String(),
		KeyID:     keyID,
		Title:     bundle.Title,
	}, nil
}

// Verify is the bundle-aware sibling of courseorders.Verify. Same
// signature-check, but enrolls the student into every course of the
// bundle on success.
func (s *Service) Verify(ctx context.Context, req VerifyRequest, userID uuid.UUID) error {
	if !s.rp.VerifyPaymentSignature(req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature) {
		return fmt.Errorf("signature mismatch")
	}

	row, err := s.q.GetCourseOrderByProviderOrderID(ctx, pgtype.Text{String: req.RazorpayOrderID, Valid: true})
	if err != nil {
		return fmt.Errorf("order not found")
	}
	if uuid.UUID(row.UserID.Bytes) != userID {
		return fmt.Errorf("not your order")
	}
	if row.Status.String == "paid" {
		return nil
	}

	// Pull the bundle id back out of the metadata jsonb.
	var meta map[string]string
	_ = json.Unmarshal(row.Metadata, &meta)
	bundleStr := meta["bundle_id"]
	if bundleStr == "" {
		return fmt.Errorf("not a bundle order")
	}
	bundleID, err := uuid.Parse(bundleStr)
	if err != nil {
		return fmt.Errorf("malformed bundle id")
	}

	if _, err := s.q.MarkCourseOrderPaid(ctx, db.MarkCourseOrderPaidParams{
		ID:                row.ID,
		ProviderPaymentID: pgtype.Text{String: req.RazorpayPaymentID, Valid: true},
		ProviderSignature: pgtype.Text{String: req.RazorpaySignature, Valid: true},
	}); err != nil {
		return err
	}

	// Fan out: enroll into every course in the bundle. Best-effort per
	// course — one failed enrollment shouldn't roll back the payment.
	items, err := s.q.ListCourseBundleItems(ctx, pgtype.UUID{Bytes: bundleID, Valid: true})
	if err != nil {
		return err
	}
	for _, courseID := range items {
		_, _ = s.q.CreateEnrollment(ctx, db.CreateEnrollmentParams{
			UserID:   row.UserID,
			CourseID: courseID,
			TenantID: row.TenantID,
		})
	}

	tenantID := uuid.UUID(row.TenantID.Bytes)
	s.producer.Emit(ctx, events.TypePaymentSucceeded, tenantID, userID, map[string]any{
		"order_id":    req.RazorpayOrderID,
		"payment_id":  req.RazorpayPaymentID,
		"bundle_id":   bundleID,
		"amount_paid": row.Amount,
	})
	for _, courseID := range items {
		s.producer.Emit(ctx, events.TypeCoursePurchased, tenantID, userID, map[string]any{
			"course_id":  uuid.UUID(courseID.Bytes),
			"via_bundle": bundleID,
		})
	}
	return nil
}

type BuyResult struct {
	OrderID   string `json:"order_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	PaymentID string `json:"payment_record_id"`
	KeyID     string `json:"key_id,omitempty"`
	Title     string `json:"title"`
}

type VerifyRequest struct {
	RazorpayOrderID   string `json:"razorpay_order_id"`
	RazorpayPaymentID string `json:"razorpay_payment_id"`
	RazorpaySignature string `json:"razorpay_signature"`
}

// decodeUUIDArray normalises Postgres uuid[] aggregate output into
// hex-string UUIDs. pgx returns these as []any of [16]byte; we don't
// import the pgtype helper because it's not exported for arrays.
func decodeUUIDArray(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(arr))
		for _, e := range arr {
			if b, ok := e.([16]byte); ok {
				out = append(out, uuid.UUID(b).String())
			} else if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case [][16]byte:
		out := make([]string, 0, len(arr))
		for _, b := range arr {
			out = append(out, uuid.UUID(b).String())
		}
		return out
	}
	return nil
}
