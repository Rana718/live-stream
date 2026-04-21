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
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q  *db.Queries
	rp *payments.Razorpay
}

func NewService(pool *pgxpool.Pool, rp *payments.Razorpay) *Service {
	return &Service{q: db.New(pool), rp: rp}
}

// --- Plans ---

type UpsertPlanRequest struct {
	Name         string   `json:"name" validate:"required"`
	Slug         string   `json:"slug" validate:"required"`
	Description  string   `json:"description"`
	Price        float64  `json:"price" validate:"gte=0"`
	Currency     string   `json:"currency"`
	DurationDays int32    `json:"duration_days" validate:"required,gt=0"`
	Features     []string `json:"features"`
	DisplayOrder int32    `json:"display_order"`
}

func (s *Service) CreatePlan(ctx context.Context, req UpsertPlanRequest) (*db.SubscriptionPlan, error) {
	if req.Currency == "" {
		req.Currency = "INR"
	}
	features, _ := json.Marshal(req.Features)
	p, err := s.q.CreateSubscriptionPlan(ctx, db.CreateSubscriptionPlanParams{
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  utils.TextToPg(req.Description),
		Price:        utils.NumericFromFloat(req.Price),
		Currency:     utils.TextToPg(req.Currency),
		DurationDays: req.DurationDays,
		Features:     features,
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) ListActivePlans(ctx context.Context) ([]db.SubscriptionPlan, error) {
	return s.q.ListActivePlans(ctx)
}

func (s *Service) GetPlan(ctx context.Context, id uuid.UUID) (*db.SubscriptionPlan, error) {
	p, err := s.q.GetPlanByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// --- Checkout / subscription flow ---

type CheckoutRequest struct {
	PlanID uuid.UUID `json:"plan_id" validate:"required"`
}

type CheckoutResponse struct {
	SubscriptionID string  `json:"subscription_id"`
	PaymentID      string  `json:"payment_id"`
	RazorpayOrder  string  `json:"razorpay_order_id"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	PublicKey      string  `json:"public_key"`
}

// StartCheckout creates a pending user_subscription + a payment row + a Razorpay order.
func (s *Service) StartCheckout(ctx context.Context, userID uuid.UUID, req CheckoutRequest, publicKey string) (*CheckoutResponse, error) {
	plan, err := s.q.GetPlanByID(ctx, utils.UUIDToPg(req.PlanID))
	if err != nil {
		return nil, fmt.Errorf("plan not found: %w", err)
	}

	sub, err := s.q.CreateUserSubscription(ctx, db.CreateUserSubscriptionParams{
		UserID:    utils.UUIDToPg(userID),
		PlanID:    plan.ID,
		Status:    utils.TextToPg("pending"),
		StartsAt:  utils.TimestampToPg(time.Time{}),
		EndsAt:    utils.TimestampToPg(time.Time{}),
		AutoRenew: utils.BoolToPg(false),
	})
	if err != nil {
		return nil, err
	}

	priceFloat := utils.NumericToFloat(plan.Price)
	if priceFloat <= 0 {
		// Free plan — activate immediately, no payment required.
		_, err := s.q.ActivateSubscription(ctx, db.ActivateSubscriptionParams{
			ID:         sub.ID,
			Days: plan.DurationDays,
		})
		if err != nil {
			return nil, err
		}
		return &CheckoutResponse{
			SubscriptionID: utils.UUIDFromPg(sub.ID),
			Amount:         0,
			Currency:       utils.TextFromPg(plan.Currency),
			PublicKey:      publicKey,
		}, nil
	}

	if s.rp == nil {
		return nil, errors.New("razorpay not configured")
	}

	amountPaise := int64(priceFloat * 100)
	receipt := fmt.Sprintf("sub_%s", utils.UUIDFromPg(sub.ID))
	order, err := s.rp.CreateOrder(ctx, amountPaise, utils.TextFromPg(plan.Currency), receipt, map[string]string{
		"user_id":         userID.String(),
		"plan_slug":       plan.Slug,
		"subscription_id": utils.UUIDFromPg(sub.ID),
	})
	if err != nil {
		return nil, err
	}

	meta, _ := json.Marshal(map[string]any{"receipt": receipt, "plan": plan.Slug})
	pay, err := s.q.CreatePayment(ctx, db.CreatePaymentParams{
		UserID:          utils.UUIDToPg(userID),
		SubscriptionID:  sub.ID,
		Amount:          utils.NumericFromFloat(priceFloat),
		Currency:        utils.TextToPg(utils.TextFromPg(plan.Currency)),
		Provider:        utils.TextToPg("razorpay"),
		ProviderOrderID: utils.TextToPg(order.ID),
		Status:          utils.TextToPg("created"),
		Metadata:        meta,
	})
	if err != nil {
		return nil, err
	}

	return &CheckoutResponse{
		SubscriptionID: utils.UUIDFromPg(sub.ID),
		PaymentID:      utils.UUIDFromPg(pay.ID),
		RazorpayOrder:  order.ID,
		Amount:         priceFloat,
		Currency:       utils.TextFromPg(plan.Currency),
		PublicKey:      publicKey,
	}, nil
}

type VerifyRequest struct {
	RazorpayOrderID   string `json:"razorpay_order_id" validate:"required"`
	RazorpayPaymentID string `json:"razorpay_payment_id" validate:"required"`
	RazorpaySignature string `json:"razorpay_signature" validate:"required"`
}

// VerifyCheckout validates the signature, activates the subscription, marks payment captured.
func (s *Service) VerifyCheckout(ctx context.Context, userID uuid.UUID, req VerifyRequest) (*db.UserSubscription, error) {
	if s.rp == nil {
		return nil, errors.New("razorpay not configured")
	}
	if !s.rp.VerifyPaymentSignature(req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature) {
		return nil, errors.New("invalid signature")
	}
	pay, err := s.q.GetPaymentByProviderOrderID(ctx, utils.TextToPg(req.RazorpayOrderID))
	if err != nil {
		return nil, fmt.Errorf("payment not found")
	}
	if utils.UUIDFromPg(pay.UserID) != userID.String() {
		return nil, errors.New("forbidden")
	}

	if _, err := s.q.UpdatePaymentStatus(ctx, db.UpdatePaymentStatusParams{
		ID:                 pay.ID,
		Status:             utils.TextToPg("captured"),
		ProviderPaymentID:  utils.TextToPg(req.RazorpayPaymentID),
		ProviderSignature:  utils.TextToPg(req.RazorpaySignature),
	}); err != nil {
		return nil, err
	}

	if !pay.SubscriptionID.Valid {
		return nil, errors.New("payment has no linked subscription")
	}
	sub, err := s.q.GetSubscriptionByID(ctx, pay.SubscriptionID)
	if err != nil {
		return nil, err
	}
	plan, err := s.q.GetPlanByID(ctx, sub.PlanID)
	if err != nil {
		return nil, err
	}
	updated, err := s.q.ActivateSubscription(ctx, db.ActivateSubscriptionParams{
		ID:           sub.ID,
		Days: plan.DurationDays,
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *Service) GetActive(ctx context.Context, userID uuid.UUID) (*db.UserSubscription, error) {
	sub, err := s.q.GetActiveSubscription(ctx, utils.UUIDToPg(userID))
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) ListMine(ctx context.Context, userID uuid.UUID) ([]db.ListUserSubscriptionsRow, error) {
	return s.q.ListUserSubscriptions(ctx, utils.UUIDToPg(userID))
}

func (s *Service) Cancel(ctx context.Context, userID, subID uuid.UUID) error {
	sub, err := s.q.GetSubscriptionByID(ctx, utils.UUIDToPg(subID))
	if err != nil {
		return err
	}
	if utils.UUIDFromPg(sub.UserID) != userID.String() {
		return errors.New("forbidden")
	}
	return s.q.CancelSubscription(ctx, utils.UUIDToPg(subID))
}

// HandleWebhook processes raw webhook payloads from Razorpay.
func (s *Service) HandleWebhook(ctx context.Context, rawBody []byte, signature string) error {
	if s.rp == nil {
		return errors.New("razorpay not configured")
	}
	if !s.rp.VerifyWebhookSignature(rawBody, signature) {
		return errors.New("invalid webhook signature")
	}
	var env struct {
		Event   string `json:"event"`
		Payload struct {
			Payment struct {
				Entity struct {
					ID      string `json:"id"`
					OrderID string `json:"order_id"`
					Status  string `json:"status"`
				} `json:"entity"`
			} `json:"payment"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(rawBody, &env); err != nil {
		return err
	}
	switch env.Event {
	case "payment.captured":
		pay, err := s.q.GetPaymentByProviderOrderID(ctx, utils.TextToPg(env.Payload.Payment.Entity.OrderID))
		if err == nil {
			_, _ = s.q.UpdatePaymentStatus(ctx, db.UpdatePaymentStatusParams{
				ID:                pay.ID,
				Status:            utils.TextToPg("captured"),
				ProviderPaymentID: utils.TextToPg(env.Payload.Payment.Entity.ID),
			})
		}
	case "payment.failed":
		pay, err := s.q.GetPaymentByProviderOrderID(ctx, utils.TextToPg(env.Payload.Payment.Entity.OrderID))
		if err == nil {
			_, _ = s.q.UpdatePaymentStatus(ctx, db.UpdatePaymentStatusParams{
				ID:                pay.ID,
				Status:            utils.TextToPg("failed"),
				ProviderPaymentID: utils.TextToPg(env.Payload.Payment.Entity.ID),
			})
		}
	}
	return nil
}
