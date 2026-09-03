package grpcserver

import (
	"context"

	billingv1 "live-platform/gen/proto/live/billing/v1"
	"live-platform/internal/billing"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"
)

type BillingServer struct {
	billingv1.UnimplementedBillingServiceServer
	svc *billing.Service
}

func NewBillingServer(svc *billing.Service) *BillingServer { return &BillingServer{svc: svc} }

func (s *BillingServer) ListMyInvoices(ctx context.Context, req *billingv1.ListMyInvoicesRequest) (*billingv1.ListMyInvoicesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListForUser(ctx, c.TenantID, c.UserID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &billingv1.ListMyInvoicesResponse{}
	for _, r := range rows {
		out.Invoices = append(out.Invoices, &billingv1.Invoice{
			Id: utils.UUIDFromPg(r.ID), Number: r.Number, OrderId: utils.UUIDFromPg(r.OrderID),
			Status: string(r.Status), SupplyType: string(r.SupplyType), TaxableMinor: r.TaxableMinor,
			CgstMinor: r.CgstMinor, SgstMinor: r.SgstMinor, IgstMinor: r.IgstMinor, TotalMinor: r.TotalMinor,
			IssuedAt: tsFromPgtz(r.IssuedAt),
		})
	}
	return out, nil
}

func (s *BillingServer) GetInvoice(ctx context.Context, req *billingv1.GetInvoiceRequest) (*billingv1.GetInvoiceResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	inv, lines, err := s.svc.GetForUser(ctx, id, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &billingv1.GetInvoiceResponse{Invoice: &billingv1.Invoice{
		Id: utils.UUIDFromPg(inv.ID), Number: inv.Number, OrderId: utils.UUIDFromPg(inv.OrderID),
		Status: string(inv.Status), SupplyType: string(inv.SupplyType), FinYear: inv.FinYear,
		TaxableMinor: inv.TaxableMinor, CgstMinor: inv.CgstMinor, SgstMinor: inv.SgstMinor, IgstMinor: inv.IgstMinor,
		RoundOffMinor: inv.RoundOffMinor, TotalMinor: inv.TotalMinor, IssuedAt: tsFromPgtz(inv.IssuedAt),
	}}
	for _, l := range lines {
		out.Lines = append(out.Lines, lineMsg(l))
	}
	return out, nil
}

func lineMsg(l db.ListInvoiceLineItemsRow) *billingv1.InvoiceLine {
	return &billingv1.InvoiceLine{
		Description: l.Description, HsnSac: utils.TextFromPg(l.HsnSac), Qty: l.Qty, UnitMinor: l.UnitMinor,
		TaxableMinor: l.TaxableMinor, RateBps: l.RateBps, CgstMinor: l.CgstMinor, SgstMinor: l.SgstMinor,
		IgstMinor: l.IgstMinor, TotalMinor: l.TotalMinor,
	}
}

func (s *BillingServer) AdminListInvoices(ctx context.Context, req *billingv1.AdminListInvoicesRequest) (*billingv1.AdminListInvoicesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListForTenant(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &billingv1.AdminListInvoicesResponse{}
	for _, r := range rows {
		out.Invoices = append(out.Invoices, &billingv1.Invoice{
			Id: utils.UUIDFromPg(r.ID), Number: r.Number, OrderId: utils.UUIDFromPg(r.OrderID),
			Status: string(r.Status), TotalMinor: r.TotalMinor, IssuedAt: tsFromPgtz(r.IssuedAt),
		})
	}
	return out, nil
}
