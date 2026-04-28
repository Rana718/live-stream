package refunds

import (
	"context"

	"live-platform/internal/database/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// PaymentRow is what the admin /refunds page renders. We keep it
// snake_case to match the rest of the API.
type PaymentRow struct {
	ID                string                 `json:"id"`
	UserID            string                 `json:"user_id"`
	FullName          string                 `json:"full_name,omitempty"`
	PhoneNumber       string                 `json:"phone_number,omitempty"`
	CourseID          string                 `json:"course_id,omitempty"`
	CourseTitle       string                 `json:"course_title,omitempty"`
	Amount            int64                  `json:"amount"`
	Currency          string                 `json:"currency"`
	Status            string                 `json:"status"`
	ProviderPaymentID string                 `json:"provider_payment_id,omitempty"`
	CreatedAt         string                 `json:"created_at"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

func (s *Service) ListPayments(ctx context.Context, limit, offset int32) ([]PaymentRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.q.AdminListPayments(ctx, db.AdminListPaymentsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	out := make([]PaymentRow, 0, len(rows))
	for _, r := range rows {
		// amount is NUMERIC(10,2) (rupees). The admin UI divides by 100
		// because `payments` rows from courseorders / coursebundles store
		// paise via the Razorpay flow. We convert to paise here so all
		// rows expose a consistent unit regardless of insert path.
		amountPaise := int64(0)
		if rupees, err := r.Amount.Float64Value(); err == nil && rupees.Valid {
			amountPaise = int64(rupees.Float64 * 100)
		}

		var meta map[string]interface{}
		if len(r.Metadata) > 0 {
			meta = decodeJSON(r.Metadata)
		}

		out = append(out, PaymentRow{
			ID:                uuid.UUID(r.ID.Bytes).String(),
			UserID:            uuid.UUID(r.UserID.Bytes).String(),
			FullName:          r.FullName.String,
			PhoneNumber:       r.PhoneNumber.String,
			CourseID:          uuidStringOrEmpty(r.CourseID),
			CourseTitle:       r.CourseTitle.String,
			Amount:            amountPaise,
			Currency:          r.Currency.String,
			Status:            r.Status.String,
			ProviderPaymentID: r.ProviderPaymentID.String,
			CreatedAt:         r.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			Metadata:          meta,
		})
	}
	return out, nil
}

// AdminListPayments — GET /admin/payments. The refunds UI uses this to
// render both the refundable list (status='paid') and the history tab
// (status='refunded'); filtering happens client-side because the result
// set per tenant is small.
func (h *Handler) AdminListPayments(c fiber.Ctx) error {
	rows, err := h.svc.ListPayments(c.Context(), 500, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}
