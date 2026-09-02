package billing

import (
	"strconv"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func pagination(c fiber.Ctx) (int32, int32) {
	limit := int32(50)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 200 {
		limit = int32(l)
	}
	offset := int32(0)
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// ListMine — GET /invoices/mine
func (h *Handler) ListMine(c fiber.Ctx) error {
	limit, offset := pagination(c)
	rows, err := h.svc.q.ListInvoicesForUser(c.Context(), db.ListInvoicesForUserParams{
		TenantID: utils.UUIDToPg(middleware.CurrentTenantID(c)),
		UserID:   utils.UUIDToPg(middleware.CurrentUserID(c)),
		Limit:    limit, Offset: offset,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "number": r.Number,
			"order_id": utils.UUIDFromPg(r.OrderID), "status": string(r.Status),
			"supply_type":   string(r.SupplyType),
			"taxable_minor": r.TaxableMinor, "cgst_minor": r.CgstMinor,
			"sgst_minor": r.SgstMinor, "igst_minor": r.IgstMinor,
			"total_minor": r.TotalMinor, "issued_at": r.IssuedAt.Time,
		}
	}
	return c.JSON(out)
}

// AdminList — GET /admin/invoices  (every invoice for the tenant)
func (h *Handler) AdminList(c fiber.Ctx) error {
	limit, offset := pagination(c)
	rows, err := h.svc.q.ListTenantInvoices(c.Context(), db.ListTenantInvoicesParams{
		TenantID: utils.UUIDToPg(middleware.CurrentTenantID(c)), Limit: limit, Offset: offset,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "number": r.Number,
			"order_id": utils.UUIDFromPg(r.OrderID), "status": string(r.Status),
			"total_minor": r.TotalMinor, "issued_at": r.IssuedAt.Time,
		}
	}
	return c.JSON(out)
}

// Get — GET /invoices/:id
func (h *Handler) Get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	r, err := h.svc.q.GetInvoiceForUser(c.Context(), db.GetInvoiceForUserParams{
		ID: utils.UUIDToPg(id), UserID: utils.UUIDToPg(middleware.CurrentUserID(c)),
	})
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
	}
	lines, _ := h.svc.q.ListInvoiceLineItems(c.Context(), r.ID)
	li := make([]fiber.Map, len(lines))
	for i, l := range lines {
		li[i] = fiber.Map{
			"description": l.Description, "hsn_sac": utils.TextFromPg(l.HsnSac),
			"qty": l.Qty, "unit_minor": l.UnitMinor, "taxable_minor": l.TaxableMinor,
			"rate_bps": l.RateBps, "cgst_minor": l.CgstMinor, "sgst_minor": l.SgstMinor,
			"igst_minor": l.IgstMinor, "total_minor": l.TotalMinor,
		}
	}
	return c.JSON(fiber.Map{
		"id": utils.UUIDFromPg(r.ID), "number": r.Number, "fin_year": r.FinYear,
		"status": string(r.Status), "supply_type": string(r.SupplyType),
		"place_of_supply": r.PlaceOfSupply,
		"buyer":           string(r.BuyerSnapshot), "seller": string(r.SellerSnapshot),
		"taxable_minor": r.TaxableMinor, "cgst_minor": r.CgstMinor,
		"sgst_minor": r.SgstMinor, "igst_minor": r.IgstMinor,
		"round_off_minor": r.RoundOffMinor, "total_minor": r.TotalMinor,
		"issued_at": r.IssuedAt.Time, "line_items": li,
	})
}
