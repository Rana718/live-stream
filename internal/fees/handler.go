package fees

import (
	"strconv"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func structureToMap(s *db.FeeStructure) fiber.Map {
	return fiber.Map{
		"id":                   utils.UUIDFromPg(s.ID),
		"course_id":            utils.UUIDFromPg(s.CourseID),
		"batch_id":             utils.UUIDFromPg(s.BatchID),
		"name":                 s.Name,
		"total_amount":         utils.NumericToFloat(s.TotalAmount),
		"currency":             utils.TextFromPg(s.Currency),
		"installments_count":   utils.Int4FromPg(s.InstallmentsCount),
		"installment_gap_days": utils.Int4FromPg(s.InstallmentGapDays),
		"is_active":            utils.BoolFromPg(s.IsActive),
	}
}

func feeToMap(f *db.StudentFee) fiber.Map {
	return fiber.Map{
		"id":              utils.UUIDFromPg(f.ID),
		"user_id":         utils.UUIDFromPg(f.UserID),
		"course_id":       utils.UUIDFromPg(f.CourseID),
		"batch_id":        utils.UUIDFromPg(f.BatchID),
		"total_amount":    utils.NumericToFloat(f.TotalAmount),
		"paid_amount":     utils.NumericToFloat(f.PaidAmount),
		"currency":        utils.TextFromPg(f.Currency),
		"status":          utils.TextFromPg(f.Status),
		"due_date":        f.DueDate,
		"created_at":      f.CreatedAt,
	}
}

func instToMap(i *db.FeeInstallment) fiber.Map {
	return fiber.Map{
		"id":                 utils.UUIDFromPg(i.ID),
		"student_fee_id":     utils.UUIDFromPg(i.StudentFeeID),
		"installment_number": i.InstallmentNumber,
		"amount":             utils.NumericToFloat(i.Amount),
		"due_date":           i.DueDate,
		"paid_at":            i.PaidAt,
		"payment_id":         utils.UUIDFromPg(i.PaymentID),
		"status":             utils.TextFromPg(i.Status),
	}
}

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit := int32(20)
	offset := int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// CreateStructure godoc
// @Summary Create a fee structure template (admin)
// @Tags fees
// @Security BearerAuth
// @Router /fees/structures [post]
func (h *Handler) CreateStructure(c fiber.Ctx) error {
	var req CreateFeeStructureRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	st, err := h.service.CreateStructure(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(structureToMap(st))
}

// ListStructuresByCourse godoc
// @Summary List active fee structures for a course
// @Tags fees
// @Router /fees/structures/course/{course_id} [get]
func (h *Handler) ListStructuresByCourse(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid course id"})
	}
	rows, err := h.service.ListStructuresByCourse(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i := range rows {
		out[i] = structureToMap(&rows[i])
	}
	return c.JSON(out)
}

// Assign godoc
// @Summary Assign a fee to a student (admin)
// @Tags fees
// @Security BearerAuth
// @Router /fees/assign [post]
func (h *Handler) Assign(c fiber.Ctx) error {
	var req AssignFeeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	sf, installments, err := h.service.Assign(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	instOut := make([]fiber.Map, len(installments))
	for i := range installments {
		instOut[i] = instToMap(&installments[i])
	}
	resp := feeToMap(sf)
	resp["installments"] = instOut
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// ListMine godoc
// @Summary List the current user's fees
// @Tags fees
// @Security BearerAuth
// @Router /fees/my [get]
func (h *Handler) ListMine(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	rows, err := h.service.ListMine(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":           utils.UUIDFromPg(r.ID),
			"course_id":    utils.UUIDFromPg(r.CourseID),
			"course_title": utils.TextFromPg(r.CourseTitle),
			"total_amount": utils.NumericToFloat(r.TotalAmount),
			"paid_amount":  utils.NumericToFloat(r.PaidAmount),
			"status":       utils.TextFromPg(r.Status),
			"due_date":     r.DueDate,
		}
	}
	return c.JSON(out)
}

// ListPending godoc
// @Summary List all pending/partial/overdue student fees (admin)
// @Tags fees
// @Security BearerAuth
// @Router /fees/pending [get]
func (h *Handler) ListPending(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListPending(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":           utils.UUIDFromPg(r.ID),
			"user_id":      utils.UUIDFromPg(r.UserID),
			"email":        r.Email,
			"full_name":    utils.TextFromPg(r.FullName),
			"total_amount": utils.NumericToFloat(r.TotalAmount),
			"paid_amount":  utils.NumericToFloat(r.PaidAmount),
			"status":       utils.TextFromPg(r.Status),
			"due_date":     r.DueDate,
		}
	}
	return c.JSON(out)
}

// ListOverdueInstallments godoc
// @Summary List overdue installments across all students (admin)
// @Tags fees
// @Security BearerAuth
// @Router /fees/installments/overdue [get]
func (h *Handler) ListOverdueInstallments(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListOverdueInstallments(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":                 utils.UUIDFromPg(r.ID),
			"student_fee_id":     utils.UUIDFromPg(r.StudentFeeID),
			"installment_number": r.InstallmentNumber,
			"amount":             utils.NumericToFloat(r.Amount),
			"due_date":           r.DueDate,
			"user_id":            utils.UUIDFromPg(r.UserID),
			"email":              r.Email,
		}
	}
	return c.JSON(out)
}

// GetInstallments godoc
// @Summary List installments for a student's fee
// @Tags fees
// @Security BearerAuth
// @Router /fees/{id}/installments [get]
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
	for i := range rows {
		out[i] = instToMap(&rows[i])
	}
	return c.JSON(out)
}

// PayInstallment godoc
// @Summary Start a Razorpay checkout for an installment
// @Tags fees
// @Security BearerAuth
// @Router /fees/installments/pay [post]
func (h *Handler) PayInstallment(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req PayInstallmentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	resp, err := h.service.StartInstallmentCheckout(c.Context(), userID, req, "")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

// VerifyInstallment godoc
// @Summary Verify Razorpay signature and mark installment paid
// @Tags fees
// @Security BearerAuth
// @Router /fees/installments/verify [post]
func (h *Handler) VerifyInstallment(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req VerifyInstallmentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.service.VerifyInstallmentPayment(c.Context(), userID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "paid"})
}

// Revenue godoc
// @Summary Revenue summary over a date range (admin dashboard)
// @Tags fees
// @Security BearerAuth
// @Router /fees/revenue [get]
func (h *Handler) Revenue(c fiber.Ctx) error {
	from := time.Now().AddDate(0, -1, 0)
	to := time.Now().AddDate(0, 0, 1)
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			from = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			to = t
		}
	}
	rep, err := h.service.Revenue(c.Context(), from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rep)
}
