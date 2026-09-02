// Package coursebundles — schema-v2. A bundle is a course_bundles row + a
// products row (kind='bundle'); its contents are bundle_items linking the
// bundle product to each course's product. Buying a bundle grants a bundle
// entitlement and fans out to per-course entitlements + enrolments.
package coursebundles

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
	pool     *pgxpool.Pool
	q        *db.Queries
	rp       *payments.Razorpay
	producer *events.Producer
	billing  *billing.Service
}

func NewService(pool *pgxpool.Pool, rp *payments.Razorpay) *Service {
	return &Service{pool: pool, q: db.New(pool), rp: rp, billing: billing.NewService(pool)}
}

func (s *Service) WithProducer(p *events.Producer) *Service { s.producer = p; return s }

func ntext(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

type BundleView struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	CoverURL    string   `json:"cover_url"`
	PricePaise  int64    `json:"price_paise"`
	PriceMinor  int64    `json:"price_minor"`
	IsActive    bool     `json:"is_active"`
	CourseIDs   []string `json:"course_ids"`
	Courses     []any    `json:"courses"`
}

func (s *Service) bundleView(ctx context.Context, tenantID uuid.UUID, id pgtype.UUID, title, desc string, cover pgtype.Text, active bool) BundleView {
	v := BundleView{
		ID: utils.UUIDFromPg(id), Title: title, Description: desc,
		CoverURL: utils.TextFromPg(cover), IsActive: active,
	}
	prod, err := s.q.GetProductForBundle(ctx, id)
	if err != nil {
		return v
	}
	if p, e := s.q.GetActivePrice(ctx, prod.ID); e == nil {
		v.PriceMinor, v.PricePaise = p.AmountMinor, p.AmountMinor
	}
	courses, _ := s.q.ListBundleCourses(ctx, prod.ID)
	for _, cr := range courses {
		v.CourseIDs = append(v.CourseIDs, utils.UUIDFromPg(cr.ID))
		v.Courses = append(v.Courses, map[string]any{
			"id": utils.UUIDFromPg(cr.ID), "title": cr.Title, "slug": cr.Slug,
			"thumbnail_url": utils.TextFromPg(cr.ThumbnailUrl),
		})
	}
	return v
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]BundleView, error) {
	rows, err := s.q.ListCourseBundles(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return nil, err
	}
	out := make([]BundleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.bundleView(ctx, tenantID, r.ID, r.Title, utils.TextFromPg(r.Description), r.CoverUrl, r.IsActive))
	}
	return out, nil
}

// ── buy / verify ────────────────────────────────────────────────────

type CreateOrderResult struct {
	OrderID   string `json:"order_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	PaymentID string `json:"payment_record_id"`
	KeyID     string `json:"key_id,omitempty"`
}

func (s *Service) Buy(ctx context.Context, tenantID, userID, bundleID uuid.UUID, keyID string) (*CreateOrderResult, error) {
	b, err := s.q.GetCourseBundle(ctx, utils.UUIDToPg(bundleID))
	if err != nil {
		return nil, fmt.Errorf("bundle not found")
	}
	prod, err := s.q.GetProductForBundle(ctx, utils.UUIDToPg(bundleID))
	if err != nil {
		return nil, fmt.Errorf("bundle not for sale")
	}
	owns, _ := s.q.CheckEntitlement(ctx, db.CheckEntitlementParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), ProductID: prod.ID,
	})
	if owns {
		return nil, fmt.Errorf("already purchased")
	}
	price, err := s.q.GetActivePrice(ctx, prod.ID)
	if err != nil || price.AmountMinor <= 0 {
		return nil, fmt.Errorf("bundle is unpriced")
	}
	total := price.AmountMinor

	seq, _ := s.q.NextOrderSequence(ctx, utils.UUIDToPg(tenantID))
	code := fmt.Sprintf("ORD-%06d", seq)
	rpOrder, err := s.rp.CreateOrder(ctx, total, "INR", code, map[string]string{
		"tenant_id": tenantID.String(), "user_id": userID.String(), "bundle_id": bundleID.String(),
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

	notes, _ := json.Marshal(map[string]string{"bundle_id": bundleID.String()})
	order, err := q.CreateOrder(ctx, db.CreateOrderParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), Code: code,
		SubtotalMinor: total, TotalMinor: total,
		Status:         db.NullOrderStatus{OrderStatus: db.OrderStatus("awaiting_payment"), Valid: true},
		Gateway:        pgtype.Text{String: "razorpay", Valid: true},
		GatewayOrderID: pgtype.Text{String: rpOrder.ID, Valid: true},
		Notes:          notes,
	})
	if err != nil {
		return nil, err
	}
	if _, err := q.CreateOrderItem(ctx, db.CreateOrderItemParams{
		TenantID: utils.UUIDToPg(tenantID), OrderID: order.ID, ProductID: prod.ID,
		ProductKind: db.ProductKind("bundle"), Title: b.Title, UnitMinor: total, Qty: 1,
		LineSubtotalMinor: total, TaxableMinor: total, TotalMinor: total,
		GrantsEntitlement: pgtype.Bool{Bool: true, Valid: true},
	}); err != nil {
		return nil, err
	}
	pay, err := q.CreatePayment(ctx, db.CreatePaymentParams{
		TenantID: utils.UUIDToPg(tenantID), OrderID: order.ID, UserID: utils.UUIDToPg(userID),
		GatewayOrderID: pgtype.Text{String: rpOrder.ID, Valid: true}, AmountMinor: total,
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

func (s *Service) Verify(ctx context.Context, req VerifyRequest, userID uuid.UUID) error {
	if !s.rp.VerifyPaymentSignature(req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature) {
		return fmt.Errorf("signature mismatch")
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
		return fmt.Errorf("not your order")
	}
	if string(order.Status) == "paid" {
		return tx.Commit(ctx)
	}
	paid, err := q.MarkOrderPaid(ctx, order.ID)
	if err != nil {
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

	items, err := q.ListOrderItems(ctx, order.ID)
	if err != nil {
		return err
	}
	for _, it := range items {
		if !it.GrantsEntitlement {
			continue
		}
		// bundle entitlement
		if _, err := q.GrantEntitlement(ctx, db.GrantEntitlementParams{
			TenantID: paid.TenantID, UserID: paid.UserID, ProductID: it.ProductID,
			ProductKind: it.ProductKind, Source: db.EntitlementSource("purchase"), OrderItemID: it.ID,
		}); err != nil {
			return err
		}
		// fan out to each course in the bundle
		contents, _ := q.ListBundleItemProducts(ctx, it.ProductID)
		for _, cp := range contents {
			if _, err := q.GrantEntitlement(ctx, db.GrantEntitlementParams{
				TenantID: paid.TenantID, UserID: paid.UserID, ProductID: cp.ID,
				ProductKind: cp.Kind, Source: db.EntitlementSource("bundle"), OrderItemID: it.ID,
			}); err != nil {
				return err
			}
			if cp.CourseID.Valid {
				if _, err := q.UpsertEnrollment(ctx, db.UpsertEnrollmentParams{
					TenantID: paid.TenantID, UserID: paid.UserID, CourseID: cp.CourseID,
				}); err != nil {
					return err
				}
			}
		}
	}
	if _, err := s.billing.GenerateForOrder(ctx, q, uuid.UUID(paid.TenantID.Bytes), uuid.UUID(order.ID.Bytes)); err != nil {
		return fmt.Errorf("invoice generation failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if s.producer != nil {
		s.producer.Emit(ctx, events.TypeCoursePurchased, uuid.UUID(paid.TenantID.Bytes), userID,
			map[string]any{"order_id": utils.UUIDFromPg(order.ID), "kind": "bundle"})
	}
	return nil
}
