// Package subscriptions — schema-v2. Plans have no price column (price lives
// on the plan's product in `prices`). Each period is a one-time order:
// checkout → order for the plan product; verify → CreateSubscription (active,
// period = plan.interval_days) + entitlement. Recurring auto-charge and the
// renewal worker are Phase F/H.
package subscriptions

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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
	rp   *payments.Razorpay
}

func NewService(pool *pgxpool.Pool, rp *payments.Razorpay) *Service {
	return &Service{pool: pool, q: db.New(pool), rp: rp}
}

func ntext(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// ── plans ───────────────────────────────────────────────────────────

type UpsertPlanRequest struct {
	Name         string   `json:"name" validate:"required"`
	Slug         string   `json:"slug" validate:"required"`
	Description  string   `json:"description"`
	Price        float64  `json:"price" validate:"gte=0"`
	PriceMinor   int64    `json:"price_minor"`
	DurationDays int32    `json:"duration_days" validate:"required,gt=0"`
	TrialDays    int32    `json:"trial_days"`
	Features     []string `json:"features"`
	DisplayOrder int32    `json:"display_order"`
}

func (r UpsertPlanRequest) priceMinor() int64 {
	if r.PriceMinor > 0 {
		return r.PriceMinor
	}
	return int64(r.Price * 100)
}

type PlanView struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Description  string   `json:"description"`
	Price        float64  `json:"price"`
	PriceMinor   int64    `json:"price_minor"`
	DurationDays int32    `json:"duration_days"`
	TrialDays    int32    `json:"trial_days"`
	Features     []string `json:"features"`
}

func (s *Service) CreatePlan(ctx context.Context, tenantID uuid.UUID, req UpsertPlanRequest) (PlanView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PlanView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	feat, _ := json.Marshal(req.Features)
	p, err := q.CreateSubscriptionPlan(ctx, db.CreateSubscriptionPlanParams{
		TenantID:     utils.UUIDToPg(tenantID),
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  ntext(req.Description),
		IntervalDays: pgtype.Int4{Int32: req.DurationDays, Valid: true},
		TrialDays:    pgtype.Int4{Int32: req.TrialDays, Valid: true},
		Features:     feat,
		DisplayOrder: pgtype.Int4{Int32: req.DisplayOrder, Valid: true},
	})
	if err != nil {
		return PlanView{}, err
	}
	prod, err := q.CreateProduct(ctx, db.CreateProductParams{
		TenantID: utils.UUIDToPg(tenantID), Kind: db.ProductKind("plan"), PlanID: p.ID,
	})
	if err != nil {
		return PlanView{}, err
	}
	if req.priceMinor() > 0 {
		if _, err := q.UpsertActivePrice(ctx, db.UpsertActivePriceParams{
			TenantID: utils.UUIDToPg(tenantID), ProductID: prod.ID, AmountMinor: req.priceMinor(),
		}); err != nil {
			return PlanView{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return PlanView{}, err
	}
	return PlanView{
		ID: utils.UUIDFromPg(p.ID), Name: p.Name, Slug: p.Slug,
		Description: req.Description, Price: float64(req.priceMinor()) / 100,
		PriceMinor: req.priceMinor(), DurationDays: p.IntervalDays,
		TrialDays: p.TrialDays, Features: req.Features,
	}, nil
}

func (s *Service) ListActivePlans(ctx context.Context, tenantID uuid.UUID) ([]PlanView, error) {
	rows, err := s.q.ListActiveSubscriptionPlans(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return nil, err
	}
	out := make([]PlanView, 0, len(rows))
	for _, r := range rows {
		v := PlanView{
			ID: utils.UUIDFromPg(r.ID), Name: r.Name, Slug: r.Slug,
			Description: utils.TextFromPg(r.Description), DurationDays: r.IntervalDays, TrialDays: r.TrialDays,
		}
		_ = json.Unmarshal(r.Features, &v.Features)
		if prod, e := s.q.GetProductForPlan(ctx, r.ID); e == nil {
			if price, e2 := s.q.GetActivePrice(ctx, prod.ID); e2 == nil {
				v.PriceMinor = price.AmountMinor
				v.Price = float64(price.AmountMinor) / 100
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) SetPlanActive(ctx context.Context, id uuid.UUID, active bool) error {
	return s.q.SetSubscriptionPlanActive(ctx, db.SetSubscriptionPlanActiveParams{
		ID: utils.UUIDToPg(id), IsActive: active,
	})
}

// ── checkout / verify ───────────────────────────────────────────────

type CheckoutRequest struct {
	PlanID uuid.UUID `json:"plan_id" validate:"required"`
}

type CheckoutResponse struct {
	OrderID       string `json:"order_id"`
	RazorpayOrder string `json:"razorpay_order_id"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	PlanName      string `json:"plan_name"`
	PublicKey     string `json:"public_key"`
}

func (s *Service) StartCheckout(ctx context.Context, tenantID, userID uuid.UUID, req CheckoutRequest, publicKey string) (*CheckoutResponse, error) {
	plan, err := s.q.GetSubscriptionPlan(ctx, utils.UUIDToPg(req.PlanID))
	if err != nil {
		return nil, fmt.Errorf("plan not found")
	}
	prod, err := s.q.GetProductForPlan(ctx, plan.ID)
	if err != nil {
		return nil, fmt.Errorf("plan not for sale")
	}
	price, err := s.q.GetActivePrice(ctx, prod.ID)
	if err != nil || price.AmountMinor <= 0 {
		return nil, fmt.Errorf("plan is unpriced")
	}
	total := price.AmountMinor

	if s.rp == nil {
		return nil, errors.New("razorpay not configured")
	}
	seq, _ := s.q.NextOrderSequence(ctx, utils.UUIDToPg(tenantID))
	code := fmt.Sprintf("ORD-%06d", seq)
	rpOrder, err := s.rp.CreateOrder(ctx, total, "INR", code, map[string]string{
		"tenant_id": tenantID.String(), "user_id": userID.String(), "plan_id": plan.ID.String(),
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

	notes, _ := json.Marshal(map[string]string{"plan_id": plan.ID.String()})
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
		ProductKind: db.ProductKind("plan"), Title: plan.Name, UnitMinor: total, Qty: 1,
		LineSubtotalMinor: total, TaxableMinor: total, TotalMinor: total,
		GrantsEntitlement: pgtype.Bool{Bool: true, Valid: true},
	}); err != nil {
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

	return &CheckoutResponse{
		OrderID: rpOrder.ID, RazorpayOrder: rpOrder.ID, Amount: total, Currency: "INR",
		PlanName: plan.Name, PublicKey: publicKey,
	}, nil
}

type VerifyRequest struct {
	RazorpayOrderID   string `json:"razorpay_order_id" validate:"required"`
	RazorpayPaymentID string `json:"razorpay_payment_id" validate:"required"`
	RazorpaySignature string `json:"razorpay_signature" validate:"required"`
}

type SubView struct {
	ID        string     `json:"id"`
	PlanID    string     `json:"plan_id"`
	Status    string     `json:"status"`
	PlanName  string     `json:"plan_name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (s *Service) VerifyCheckout(ctx context.Context, userID uuid.UUID, req VerifyRequest) (*SubView, error) {
	if s.rp == nil || !s.rp.VerifyPaymentSignature(req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature) {
		return nil, errors.New("invalid signature")
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
		return nil, errors.New("forbidden")
	}

	planID := pgtype.UUID{}
	items, _ := q.ListOrderItems(ctx, order.ID)
	var entID pgtype.UUID
	if string(order.Status) != "paid" {
		if _, err := q.MarkOrderPaid(ctx, order.ID); err != nil {
			return nil, err
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
		for _, it := range items {
			if string(it.ProductKind) != "plan" {
				continue
			}
			p, e := q.GetProduct(ctx, it.ProductID)
			if e != nil || !p.PlanID.Valid {
				continue
			}
			planID = p.PlanID
			ent, e2 := q.GrantEntitlement(ctx, db.GrantEntitlementParams{
				TenantID: order.TenantID, UserID: order.UserID, ProductID: it.ProductID,
				ProductKind: db.ProductKind("plan"), Source: db.EntitlementSource("subscription"), OrderItemID: it.ID,
			})
			if e2 == nil {
				entID = ent.ID
			}
		}
	}

	// Period from plan.interval_days.
	days := int32(30)
	name := "Subscription"
	if planID.Valid {
		if pl, e := q.GetSubscriptionPlan(ctx, planID); e == nil {
			days, name = pl.IntervalDays, pl.Name
		}
	}
	now := time.Now()
	sub, err := q.CreateSubscription(ctx, db.CreateSubscriptionParams{
		TenantID:           order.TenantID,
		UserID:             order.UserID,
		PlanID:             planID,
		Status:             db.SubscriptionStatus("active"),
		CurrentPeriodStart: pgtype.Timestamptz{Time: now, Valid: true},
		CurrentPeriodEnd:   pgtype.Timestamptz{Time: now.AddDate(0, 0, int(days)), Valid: true},
		OriginOrderID:      order.ID,
		EntitlementID:      entID,
	})
	if err != nil {
		return nil, err
	}
	// entitlement expiry = period end
	if entID.Valid {
		_ = q.ExtendEntitlement(ctx, db.ExtendEntitlementParams{
			ID: entID, ExpiresAt: pgtype.Timestamptz{Time: now.AddDate(0, 0, int(days)), Valid: true},
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	end := now.AddDate(0, 0, int(days))
	return &SubView{
		ID: utils.UUIDFromPg(sub.ID), PlanID: utils.UUIDFromPg(planID),
		Status: string(sub.Status), PlanName: name, ExpiresAt: &end,
	}, nil
}

func (s *Service) GetActive(ctx context.Context, tenantID, userID uuid.UUID) (*SubView, error) {
	row, err := s.q.GetActiveSubscriptionForUser(ctx, db.GetActiveSubscriptionForUserParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
	if err != nil {
		return nil, err
	}
	v := &SubView{
		ID: utils.UUIDFromPg(row.ID), PlanID: utils.UUIDFromPg(row.PlanID), Status: string(row.Status),
	}
	if row.CurrentPeriodEnd.Valid {
		t := row.CurrentPeriodEnd.Time
		v.ExpiresAt = &t
	}
	if row.PlanID.Valid {
		if pl, e := s.q.GetSubscriptionPlan(ctx, row.PlanID); e == nil {
			v.PlanName = pl.Name
		}
	}
	return v, nil
}

type HistoryRow struct {
	ID       string `json:"id"`
	PlanID   string `json:"plan_id"`
	PlanName string `json:"plan_name"`
	Status   string `json:"status"`
}

func (s *Service) ListMine(ctx context.Context, tenantID, userID uuid.UUID) ([]HistoryRow, error) {
	rows, err := s.q.ListSubscriptionsForUser(ctx, db.ListSubscriptionsForUserParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]HistoryRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, HistoryRow{
			ID: utils.UUIDFromPg(r.ID), PlanID: utils.UUIDFromPg(r.PlanID),
			PlanName: r.PlanName, Status: string(r.Status),
		})
	}
	return out, nil
}

func (s *Service) Cancel(ctx context.Context, tenantID, userID, subID uuid.UUID) error {
	return s.q.SetSubscriptionStatus(ctx, db.SetSubscriptionStatusParams{
		ID: utils.UUIDToPg(subID), Status: db.SubscriptionStatus("cancelled"),
	})
}

// HandleWebhook — moved to internal/webhooks in Phase G. Kept as a no-op so
// the route stays wired.
func (s *Service) HandleWebhook(ctx context.Context, rawBody []byte, signature string) error {
	return nil
}
