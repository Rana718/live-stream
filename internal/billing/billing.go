// Package billing generates GST invoices (immutable, gapless per-tenant
// per-financial-year numbering) and credit notes. Invoice creation runs
// inside the same transaction that captures the payment so numbering never
// leaves a gap on a happy path; a rollback is GST-acceptable ("cancelled
// via credit note").
package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/money"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, q: db.New(pool)} }

// --- read side (shared by the REST handler and the gRPC adapter) ---

func (s *Service) ListForUser(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]db.ListInvoicesForUserRow, error) {
	return s.q.ListInvoicesForUser(ctx, db.ListInvoicesForUserParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), Limit: limit, Offset: offset,
	})
}

func (s *Service) ListForTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.ListTenantInvoicesRow, error) {
	return s.q.ListTenantInvoices(ctx, db.ListTenantInvoicesParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
}

func (s *Service) GetForUser(ctx context.Context, id, userID uuid.UUID) (db.GetInvoiceForUserRow, []db.ListInvoiceLineItemsRow, error) {
	inv, err := s.q.GetInvoiceForUser(ctx, db.GetInvoiceForUserParams{
		ID: utils.UUIDToPg(id), UserID: utils.UUIDToPg(userID),
	})
	if err != nil {
		return inv, nil, err
	}
	lines, err := s.q.ListInvoiceLineItems(ctx, inv.ID)
	return inv, lines, err
}

// finYear returns the Indian financial year string ("2026-27") for t.
func finYear(t time.Time) string {
	y := t.Year()
	if t.Month() < time.April {
		y--
	}
	return fmt.Sprintf("%d-%02d", y, (y+1)%100)
}

// GenerateForOrder creates the tax invoice for a paid order, using qtx so it
// joins the capture transaction. Idempotent: returns the existing invoice id
// if one already exists for the order. Prices on order_items are treated as
// GST-inclusive.
func (s *Service) GenerateForOrder(ctx context.Context, qtx *db.Queries, tenantID, orderID uuid.UUID) (uuid.UUID, error) {
	if existing, err := qtx.GetInvoiceByOrder(ctx, utils.UUIDToPg(orderID)); err == nil {
		return uuid.UUID(existing.ID.Bytes), nil
	}

	order, err := qtx.GetOrderByID(ctx, utils.UUIDToPg(orderID))
	if err != nil {
		return uuid.Nil, fmt.Errorf("billing: order not found: %w", err)
	}
	items, err := qtx.ListOrderItems(ctx, utils.UUIDToPg(orderID))
	if err != nil {
		return uuid.Nil, err
	}
	tenant, err := qtx.GetTenantByID(ctx, utils.UUIDToPg(tenantID))
	if err != nil {
		return uuid.Nil, err
	}
	buyer, _ := qtx.GetUserByID(ctx, order.UserID)

	sellerState := utils.TextFromPg(tenant.PlaceOfSupply)
	buyerState := utils.TextFromPg(order.PlaceOfSupply)
	if buyerState == "" {
		buyerState = sellerState // assume intra-state when the buyer gave no state
	}
	interState := sellerState != "" && buyerState != "" && sellerState != buyerState
	supplyType := db.GstSupplyType("intra_state")
	if interState {
		supplyType = db.GstSupplyType("inter_state")
	}

	fy := finYear(time.Now())
	if err := qtx.EnsureInvoiceSeries(ctx, db.EnsureInvoiceSeriesParams{
		TenantID: utils.UUIDToPg(tenantID), FinYear: fy,
	}); err != nil {
		return uuid.Nil, err
	}
	num, err := qtx.AllocateInvoiceNumber(ctx, db.AllocateInvoiceNumberParams{
		TenantID: utils.UUIDToPg(tenantID), FinYear: fy,
	})
	if err != nil {
		return uuid.Nil, err
	}
	number := fmt.Sprintf("%s/%s/%06d", num.Prefix, fy, num.Seq)

	var taxable, cgst, sgst, igst money.Money
	taxable = money.Zero(money.INR)
	cgst, sgst, igst = money.Zero(money.INR), money.Zero(money.INR), money.Zero(money.INR)

	type lineCalc struct {
		desc    string
		hsn     string
		qty     int32
		unit    int64
		rateBps int64
		g       money.GST
	}
	lines := make([]lineCalc, 0, len(items))
	for _, it := range items {
		gross := money.New(it.TotalMinor, money.INR)
		rate := rateForItem(ctx, qtx, tenantID, it.HsnSac, tenant)
		g := money.SplitGSTInclusive(gross, rate, interState)
		taxable = taxable.Add(g.Taxable)
		cgst = cgst.Add(g.CGST)
		sgst = sgst.Add(g.SGST)
		igst = igst.Add(g.IGST)
		lines = append(lines, lineCalc{
			desc: it.Title, hsn: utils.TextFromPg(it.HsnSac), qty: it.Qty,
			unit: it.UnitMinor, rateBps: rate, g: g,
		})
	}

	componentsTotal := taxable.Add(cgst).Add(sgst).Add(igst)
	rounded, roundOff := money.New(order.TotalMinor, money.INR).RoundToRupee()
	_ = rounded
	// round_off is the delta between the invoice grand total and the sum of
	// its components (kept small — sub-rupee).
	roundOff = money.New(order.TotalMinor, money.INR).Sub(componentsTotal)

	buyerSnap := snapshotBuyer(buyer, buyerState)
	sellerSnap := snapshotSeller(tenant)

	inv, err := qtx.CreateInvoice(ctx, db.CreateInvoiceParams{
		TenantID:       utils.UUIDToPg(tenantID),
		OrderID:        utils.UUIDToPg(orderID),
		Number:         number,
		FinYear:        fy,
		SupplyType:     supplyType,
		PlaceOfSupply:  buyerState,
		BuyerSnapshot:  buyerSnap,
		SellerSnapshot: sellerSnap,
		TaxableMinor:   taxable.Minor,
		CgstMinor:      cgst.Minor,
		SgstMinor:      sgst.Minor,
		IgstMinor:      igst.Minor,
		RoundOffMinor:  pgtype.Int8{Int64: roundOff.Minor, Valid: true},
		TotalMinor:     order.TotalMinor,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("billing: create invoice: %w", err)
	}
	for _, l := range lines {
		if err := qtx.CreateInvoiceLineItem(ctx, db.CreateInvoiceLineItemParams{
			TenantID:     utils.UUIDToPg(tenantID),
			InvoiceID:    inv.ID,
			Description:  l.desc,
			HsnSac:       utils.TextToPg(l.hsn),
			Qty:          l.qty,
			UnitMinor:    l.unit,
			TaxableMinor: l.g.Taxable.Minor,
			RateBps:      int32(l.rateBps),
			CgstMinor:    l.g.CGST.Minor,
			SgstMinor:    l.g.SGST.Minor,
			IgstMinor:    l.g.IGST.Minor,
			TotalMinor:   l.g.Total.Minor,
		}); err != nil {
			return uuid.Nil, err
		}
	}
	return uuid.UUID(inv.ID.Bytes), nil
}

// GenerateForRefund issues a credit note against the order's invoice for a
// (possibly partial) refund amount, reversing GST proportionally.
func (s *Service) GenerateForRefund(ctx context.Context, qtx *db.Queries, tenantID, orderID, refundID uuid.UUID, amountMinor int64, reason string) (uuid.UUID, error) {
	if cn, err := qtx.GetCreditNoteByRefund(ctx, utils.UUIDToPg(refundID)); err == nil {
		return uuid.UUID(cn.ID.Bytes), nil
	}
	inv, err := qtx.GetInvoiceByOrder(ctx, utils.UUIDToPg(orderID))
	if err != nil {
		return uuid.Nil, fmt.Errorf("billing: no invoice for order: %w", err)
	}
	interState := string(inv.SupplyType) == "inter_state"

	// Proportional GST on the refunded slice of the invoice total.
	rate := int64(0)
	if inv.TaxableMinor > 0 {
		rate = (inv.CgstMinor + inv.SgstMinor + inv.IgstMinor) * 10000 / inv.TaxableMinor
	}
	g := money.SplitGSTInclusive(money.New(amountMinor, money.INR), rate, interState)

	fy := finYear(time.Now())
	if err := qtx.EnsureCreditNoteSeries(ctx, db.EnsureCreditNoteSeriesParams{
		TenantID: utils.UUIDToPg(tenantID), FinYear: fy,
	}); err != nil {
		return uuid.Nil, err
	}
	num, err := qtx.AllocateCreditNoteNumber(ctx, db.AllocateCreditNoteNumberParams{
		TenantID: utils.UUIDToPg(tenantID), FinYear: fy,
	})
	if err != nil {
		return uuid.Nil, err
	}
	number := fmt.Sprintf("%s/%s/%06d", num.Prefix, fy, num.Seq)

	cn, err := qtx.CreateCreditNote(ctx, db.CreateCreditNoteParams{
		TenantID:     utils.UUIDToPg(tenantID),
		InvoiceID:    inv.ID,
		RefundID:     utils.UUIDToPg(refundID),
		Number:       number,
		FinYear:      fy,
		Reason:       utils.TextToPg(reason),
		TaxableMinor: g.Taxable.Minor,
		CgstMinor:    g.CGST.Minor,
		SgstMinor:    g.SGST.Minor,
		IgstMinor:    g.IGST.Minor,
		TotalMinor:   amountMinor,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("billing: create credit note: %w", err)
	}
	_ = qtx.CreateCreditNoteLineItem(ctx, db.CreateCreditNoteLineItemParams{
		TenantID:     utils.UUIDToPg(tenantID),
		CreditNoteID: cn.ID,
		Description:  "Refund: " + reason,
		Qty:          1,
		UnitMinor:    amountMinor,
		TaxableMinor: g.Taxable.Minor,
		RateBps:      int32(rate),
		CgstMinor:    g.CGST.Minor,
		SgstMinor:    g.SGST.Minor,
		IgstMinor:    g.IGST.Minor,
		TotalMinor:   amountMinor,
	})
	return uuid.UUID(cn.ID.Bytes), nil
}

func rateForItem(ctx context.Context, qtx *db.Queries, tenantID uuid.UUID, hsn pgtype.Text, tenant db.GetTenantByIDRow) int64 {
	code := utils.TextFromPg(hsn)
	if code == "" {
		code = "9992" // default education SAC
	}
	r, err := qtx.GetTaxRate(ctx, db.GetTaxRateParams{HsnSac: code, TenantID: utils.UUIDToPg(tenantID)})
	if err != nil {
		return 0
	}
	return int64(r)
}

func snapshotBuyer(u db.GetUserByIDRow, state string) []byte {
	b, _ := json.Marshal(map[string]any{
		"name":            utils.TextFromPg(u.FullName),
		"email":           utils.TextFromPg(u.Email),
		"phone":           utils.TextFromPg(u.Phone),
		"place_of_supply": state,
	})
	return b
}

func snapshotSeller(t db.GetTenantByIDRow) []byte {
	b, _ := json.Marshal(map[string]any{
		"name":            t.Name,
		"legal_name":      utils.TextFromPg(t.LegalName),
		"gstin":           utils.TextFromPg(t.Gstin),
		"pan":             utils.TextFromPg(t.Pan),
		"place_of_supply": utils.TextFromPg(t.PlaceOfSupply),
		"address":         t.RegisteredAddress,
	})
	return b
}

var _ = pgx.ErrNoRows
