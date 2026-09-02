package refunds

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type PaymentRow struct {
	ID             string `json:"id"`
	OrderID        string `json:"order_id"`
	OrderCode      string `json:"order_code"`
	UserID         string `json:"user_id"`
	FullName       string `json:"full_name,omitempty"`
	Phone          string `json:"phone,omitempty"`
	Email          string `json:"email,omitempty"`
	Amount         int64  `json:"amount"`
	AmountMinor    int64  `json:"amount_minor"`
	RefundedMinor  int64  `json:"refunded_minor"`
	Currency       string `json:"currency"`
	Status         string `json:"status"`
	GatewayPayment string `json:"provider_payment_id,omitempty"`
	CreatedAt      string `json:"created_at"`
}

func (s *Service) ListPayments(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]PaymentRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.q.AdminListPayments(ctx, db.AdminListPaymentsParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]PaymentRow, 0, len(rows))
	for _, r := range rows {
		created := ""
		if r.CreatedAt.Valid {
			created = r.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, PaymentRow{
			ID: utils.UUIDFromPg(r.ID), OrderID: utils.UUIDFromPg(r.OrderID), OrderCode: r.OrderCode,
			UserID: utils.UUIDFromPg(r.UserID), FullName: utils.TextFromPg(r.FullName),
			Phone: utils.TextFromPg(r.Phone), Email: utils.TextFromPg(r.Email),
			Amount: r.AmountMinor, AmountMinor: r.AmountMinor, RefundedMinor: r.RefundedMinor,
			Currency: r.Currency, Status: string(r.Status),
			GatewayPayment: utils.TextFromPg(r.GatewayPaymentID), CreatedAt: created,
		})
	}
	return out, nil
}

// AdminListPayments — GET /admin/payments
func (h *Handler) AdminListPayments(c fiber.Ctx) error {
	rows, err := h.svc.ListPayments(c.Context(), middleware.CurrentTenantID(c), 500, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}
