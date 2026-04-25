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

	"live-platform/internal/database/db"
	"live-platform/internal/payments"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	q   *db.Queries
	rp  *payments.Razorpay
	log *slog.Logger
}

func NewHandler(pool *pgxpool.Pool, rp *payments.Razorpay, log *slog.Logger) *Handler {
	return &Handler{q: db.New(pool), rp: rp, log: log}
}

type rzpEnvelope struct {
	Event   string `json:"event"`
	Payload struct {
		Payment struct {
			Entity struct {
				ID       string `json:"id"`
				OrderID  string `json:"order_id"`
				Status   string `json:"status"`
				Notes    map[string]string `json:"notes"`
				Currency string `json:"currency"`
				Amount   int64  `json:"amount"`
			} `json:"entity"`
		} `json:"payment"`
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
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	var env rzpEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	switch env.Event {
	case "payment.captured", "payment.authorized":
		if err := h.applyPaymentSuccess(c.Context(), env); err != nil {
			h.log.Error("webhook apply failed",
				slog.String("event", env.Event),
				slog.String("err", err.Error()))
			// Razorpay retries 5xx — return 500 only when WE want a retry.
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	case "payment.failed":
		// Could mark the row as failed; for now just log so we have the
		// audit trail without bothering the user with a notification.
		h.log.Info("razorpay payment failed",
			slog.String("order_id", env.Payload.Payment.Entity.OrderID))
	}
	// Always 200 on unknown events so Razorpay stops retrying.
	return c.SendStatus(fiber.StatusOK)
}

func (h *Handler) applyPaymentSuccess(ctx context.Context, env rzpEnvelope) error {
	orderID := env.Payload.Payment.Entity.OrderID
	if orderID == "" {
		return fmt.Errorf("missing order_id")
	}

	row, err := h.q.GetCourseOrderByProviderOrderID(ctx, pgtype.Text{String: orderID, Valid: true})
	if err != nil {
		// Not all webhooks are course orders — could be a subscription.
		// Silently skip.
		return nil
	}

	// Idempotency — already processed by client-side verify path.
	if row.Status.String == "paid" {
		return nil
	}

	updated, err := h.q.MarkCourseOrderPaid(ctx, db.MarkCourseOrderPaidParams{
		ID:                row.ID,
		ProviderPaymentID: pgtype.Text{String: env.Payload.Payment.Entity.ID, Valid: true},
		ProviderSignature: pgtype.Text{String: "webhook", Valid: true},
	})
	if err != nil {
		return err
	}

	// Auto-enroll just like the client-side verify path.
	_, _ = h.q.CreateEnrollment(ctx, db.CreateEnrollmentParams{
		UserID:   updated.UserID,
		CourseID: updated.CourseID,
		TenantID: updated.TenantID,
	})

	h.log.Info("razorpay webhook settled order",
		slog.String("order_id", orderID),
		slog.String("user_id", uuid.UUID(updated.UserID.Bytes).String()))
	return nil
}
