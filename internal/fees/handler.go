package fees

import (
	"strconv"
	"time"

	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handler struct {
	service   *Service
	publicKey string
}

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

// WithPublicKey wires the Razorpay key id returned to the checkout client.
func (h *Handler) WithPublicKey(k string) *Handler { h.publicKey = k; return h }

func (h *Handler) tenant(c fiber.Ctx) uuid.UUID { return middleware.CurrentTenantID(c) }
func (h *Handler) user(c fiber.Ctx) uuid.UUID   { return middleware.CurrentUserID(c) }

func money(m int64) float64 { return float64(m) / 100 }
func dval(d pgtype.Date) any {
	if !d.Valid {
		return nil
	}
	return d.Time.Format("2006-01-02")
}
func tval(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit, offset := int32(50), int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 200 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// CreateStructure — POST /fees/structures  (admin)
func (h *Handler) CreateStructure(c fiber.Ctx) error {
	var req CreateFeeStructureRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	p, err := h.service.CreateStructure(c.Context(), h.tenant(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": utils.UUIDFromPg(p.ID), "name": p.Name, "total_amount": money(p.TotalMinor),
		"total_minor": p.TotalMinor, "installments_count": p.InstallmentsCount,
		"gap_days": p.GapDays, "is_active": p.IsActive,
	})
}

// ListStructuresByCourse — GET /fees/structures/course/:course_id
func (h *Handler) ListStructuresByCourse(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	rows, err := h.service.ListStructuresByCourse(c.Context(), h.tenant(c), courseID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "name": r.Name, "total_amount": money(r.TotalMinor),
			"total_minor": r.TotalMinor, "installments_count": r.InstallmentsCount,
			"gap_days": r.GapDays, "is_active": r.IsActive,
		}
	}
	return c.JSON(out)
}

// Assign — POST /fees/assign  (admin)
func (h *Handler) Assign(c fiber.Ctx) error {
	var req AssignFeeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	acc, insts, err := h.service.Assign(c.Context(), h.tenant(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(insts))
	for i, it := range insts {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(it.ID), "seq": it.Seq, "amount": money(it.AmountMinor),
			"due_date": dval(it.DueOn), "status": string(it.Status),
		}
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"fee_account_id": utils.UUIDFromPg(acc.ID), "total_amount": money(acc.TotalMinor),
		"status": string(acc.Status), "installments": out,
	})
}

// ListMine — GET /fees/my  (student: installments)
func (h *Handler) ListMine(c fiber.Ctx) error {
	rows, err := h.service.ListMine(c.Context(), h.tenant(c), h.user(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		st := string(r.Status)
		if st == "pending" && r.DueOn.Valid && r.DueOn.Time.Before(time.Now()) {
			st = "overdue"
		}
		out[i] = fiber.Map{
			"installment_id": utils.UUIDFromPg(r.ID),
			"fee_id":         utils.UUIDFromPg(r.FeeAccountID),
			"course_title":   utils.TextFromPg(r.CourseTitle),
			"seq":            r.Seq,
			"amount":         money(r.AmountMinor),
			"due_date":       dval(r.DueOn),
			"status":         st,
			"paid_at":        tval(r.PaidAt),
		}
	}
	return c.JSON(out)
}

// ListPending — GET /fees/pending  (admin)
func (h *Handler) ListPending(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListPending(c.Context(), h.tenant(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "user_id": utils.UUIDFromPg(r.UserID),
			"full_name": utils.TextFromPg(r.FullName), "phone": utils.TextFromPg(r.Phone),
			"total_amount": money(r.TotalMinor), "paid_amount": money(r.PaidMinor),
			"status": string(r.Status), "due_date": dval(r.DueOn),
		}
	}
	return c.JSON(out)
}

// ListOverdueInstallments — GET /fees/installments/overdue  (admin)
func (h *Handler) ListOverdueInstallments(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListOverdueInstallments(c.Context(), h.tenant(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "fee_account_id": utils.UUIDFromPg(r.FeeAccountID),
			"seq": r.Seq, "amount": money(r.AmountMinor), "due_date": dval(r.DueOn),
			"user_id": utils.UUIDFromPg(r.UserID), "full_name": utils.TextFromPg(r.FullName),
			"phone": utils.TextFromPg(r.Phone),
		}
	}
	return c.JSON(out)
}

// GetInstallments — GET /fees/:id/installments
func (h *Handler) GetInstallments(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	rows, err := h.service.GetInstallments(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "seq": r.Seq, "amount": money(r.AmountMinor),
			"due_date": dval(r.DueOn), "status": string(r.Status), "paid_at": tval(r.PaidAt),
		}
	}
	return c.JSON(out)
}

// PayInstallment — POST /fees/installments/pay
func (h *Handler) PayInstallment(c fiber.Ctx) error {
	var req PayInstallmentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	resp, err := h.service.StartInstallmentCheckout(c.Context(), h.tenant(c), h.user(c), req, h.publicKey)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

// VerifyInstallment — POST /fees/installments/verify
func (h *Handler) VerifyInstallment(c fiber.Ctx) error {
	var req VerifyInstallmentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.service.VerifyInstallmentPayment(c.Context(), h.user(c), req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "payment verified"})
}

// Revenue — GET /fees/revenue  (admin)
func (h *Handler) Revenue(c fiber.Ctx) error {
	from := time.Now().AddDate(0, -1, 0)
	to := time.Now()
	r, err := h.service.Revenue(c.Context(), from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(r)
}
