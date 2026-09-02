// Package webhooks implements idempotent backstop processing for Razorpay
// callbacks. Most of the time the client-side /payments/verify path settles
// the order; webhooks handle the cases where the client never returned
// (closed browser, network drop, mobile background-kill).
package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"live-platform/internal/billing"
	"live-platform/internal/database"
	"live-platform/internal/database/db"
	"live-platform/internal/metrics"
	"live-platform/internal/payments"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	rp      *payments.Razorpay
	log     *slog.Logger
	billing *billing.Service
}

func NewHandler(pool *pgxpool.Pool, rp *payments.Razorpay, log *slog.Logger) *Handler {
	return &Handler{pool: pool, q: db.New(pool), rp: rp, log: log, billing: billing.NewService(pool)}
}

type rzpEnvelope struct {
	Event   string `json:"event"`
	Payload struct {
		Payment struct {
			Entity struct {
				ID       string            `json:"id"`
				OrderID  string            `json:"order_id"`
				Status   string            `json:"status"`
				Notes    map[string]string `json:"notes"`
				Currency string            `json:"currency"`
				Amount   int64             `json:"amount"`
			} `json:"entity"`
		} `json:"payment"`
		Subscription struct {
			Entity struct {
				ID         string            `json:"id"`
				Status     string            `json:"status"`
				CurrentEnd int64             `json:"current_end"`
				Notes      map[string]string `json:"notes"`
			} `json:"entity"`
		} `json:"subscription"`
	} `json:"payload"`
}

// Razorpay handles POST /api/v1/webhooks/razorpay.
//
// Auth: signature header (X-Razorpay-Signature) verified against the raw
// request body using the webhook secret.
//
//	@Summary	Razorpay webhook callback
//	@Tags		webhooks
//	@Router		/webhooks/razorpay [post]
func (h *Handler) Razorpay(c fiber.Ctx) error {
	body, err := io.ReadAll(c.Request().BodyStream())
	if err != nil || len(body) == 0 {
		body = c.Body() // fallback when not streamed
	}

	signature := c.Get("X-Razorpay-Signature")
	if !h.rp.VerifyWebhookSignature(body, signature) {
		h.log.Warn("razorpay webhook signature mismatch")
		metrics.PaymentWebhookEvents.WithLabelValues("unknown", "rejected").Inc()
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	var env rzpEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		metrics.PaymentWebhookEvents.WithLabelValues("unknown", "rejected").Inc()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	// This is a trusted, signature-verified server-to-server call. It has no
	// authenticated user and can't know the tenant until it reads the order
	// row, so it runs cross-tenant (RLS bypass). Without this every query
	// below silently matches zero rows.
	ctx := database.WithSuperAdmin(c.Context())

	switch env.Event {
	case "payment.captured":
		// Only a *captured* payment means money actually moved. An
		// "authorized" payment can still be voided/expire, so we never
		// grant access on payment.authorized alone.
		if err := h.applyPaymentSuccess(ctx, env); err != nil {
			h.log.Error("webhook apply failed",
				slog.String("event", env.Event),
				slog.String("err", err.Error()))
			metrics.PaymentWebhookEvents.WithLabelValues(env.Event, "error").Inc()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		metrics.PaymentWebhookEvents.WithLabelValues(env.Event, "applied").Inc()
	case "payment.authorized":
		h.log.Info("razorpay payment authorized (awaiting capture)",
			slog.String("order_id", env.Payload.Payment.Entity.OrderID))
	case "payment.failed":
		h.log.Info("razorpay payment failed",
			slog.String("order_id", env.Payload.Payment.Entity.OrderID))

	// Subscription lifecycle — patch user_subscriptions keyed on the
	// razorpay_subscription_id we stored at create time. Missing rows
	// are logged but not fatal; the subscription might belong to a flow
	// we haven't mapped yet (legacy data, manual setup).
	case "subscription.activated", "subscription.charged",
		"subscription.cancelled", "subscription.completed", "subscription.halted":
		subID := env.Payload.Subscription.Entity.ID
		status := db.SubscriptionStatus("active")
		switch env.Event {
		case "subscription.cancelled":
			status = db.SubscriptionStatus("cancelled")
		case "subscription.completed":
			status = db.SubscriptionStatus("expired")
		case "subscription.halted":
			status = db.SubscriptionStatus("past_due")
		}
		if subID != "" {
			if row, err := h.q.GetSubscriptionByGatewayID(ctx, subID); err == nil {
				_ = h.q.SetSubscriptionStatus(ctx, db.SetSubscriptionStatusParams{ID: row.ID, Status: status})
			} else {
				h.log.Warn("subscription not found for gateway id", slog.String("subscription_id", subID))
			}
		}
	case "refund.created", "refund.processed":
		h.log.Info("razorpay refund",
			slog.String("payment_id", env.Payload.Payment.Entity.ID))
	}
	// Always 200 on unknown events so Razorpay stops retrying.
	return c.SendStatus(fiber.StatusOK)
}

func (h *Handler) applyPaymentSuccess(ctx context.Context, env rzpEnvelope) error {
	orderID := env.Payload.Payment.Entity.OrderID
	if orderID == "" {
		return fmt.Errorf("missing order_id")
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.q.WithTx(tx)

	order, err := qtx.GetOrderByGatewayOrderIDForUpdate(ctx, orderID)
	if err != nil {
		// Not all webhooks map to an order we recognise. Skip (not an error).
		return nil
	}
	if string(order.Status) == "paid" || string(order.Status) == "refunded" {
		return nil // already settled
	}

	paid, err := qtx.MarkOrderPaid(ctx, order.ID)
	if err != nil {
		return err
	}
	pays, _ := qtx.ListPaymentsForOrder(ctx, order.ID)
	for _, p := range pays {
		if string(p.Status) == "created" || string(p.Status) == "authorized" {
			_, _ = qtx.MarkPaymentCaptured(ctx, db.MarkPaymentCapturedParams{
				ID:               p.ID,
				GatewayPaymentID: pgtype.Text{String: env.Payload.Payment.Entity.ID, Valid: true},
				Signature:        pgtype.Text{String: "webhook", Valid: true},
			})
			break
		}
	}

	items, err := qtx.ListOrderItems(ctx, order.ID)
	if err != nil {
		return err
	}
	for _, it := range items {
		if !it.GrantsEntitlement {
			continue
		}
		src := db.EntitlementSource("purchase")
		if string(it.ProductKind) == "plan" {
			src = db.EntitlementSource("subscription")
		}
		if _, err := qtx.GrantEntitlement(ctx, db.GrantEntitlementParams{
			TenantID: paid.TenantID, UserID: paid.UserID, ProductID: it.ProductID,
			ProductKind: it.ProductKind, Source: src, OrderItemID: it.ID,
		}); err != nil {
			return fmt.Errorf("grant on webhook: %w", err)
		}
		if string(it.ProductKind) == "course" {
			if pr, e := qtx.GetProduct(ctx, it.ProductID); e == nil && pr.CourseID.Valid {
				if _, err := qtx.UpsertEnrollment(ctx, db.UpsertEnrollmentParams{
					TenantID: paid.TenantID, UserID: paid.UserID, CourseID: pr.CourseID,
				}); err != nil {
					return fmt.Errorf("enroll on webhook: %w", err)
				}
			}
		}
	}

	if _, err := h.billing.GenerateForOrder(ctx, qtx, uuid.UUID(paid.TenantID.Bytes), uuid.UUID(order.ID.Bytes)); err != nil {
		return fmt.Errorf("invoice on webhook: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.log.Info("razorpay webhook settled order",
		slog.String("order_id", orderID),
		slog.String("user_id", uuid.UUID(paid.UserID.Bytes).String()))
	return nil
}
