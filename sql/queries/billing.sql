-- billing.sql — GST invoices (immutable, gapless per-tenant per-FY numbering),
-- credit notes, and tax-rate lookup. Number allocation locks the series row.

-- ─────────────────────────────────────────────────────────── tax_rates

-- name: GetTaxRate :one
-- Effective platform-or-tenant rate for an HSN/SAC. Tenant override wins.
SELECT rate_bps
FROM tax_rates
WHERE hsn_sac = $1
  AND (tenant_id = sqlc.narg(tenant_id)::uuid OR tenant_id IS NULL)
  AND effective_from <= current_date
ORDER BY (tenant_id IS NOT NULL) DESC, effective_from DESC
LIMIT 1;

-- name: ListTenantTaxRates :many
SELECT id, hsn_sac, name, rate_bps, effective_from
FROM tax_rates
WHERE tenant_id = $1 OR tenant_id IS NULL
ORDER BY hsn_sac, effective_from DESC;

-- name: UpsertTenantTaxRate :one
INSERT INTO tax_rates (tenant_id, hsn_sac, name, rate_bps, effective_from)
VALUES ($1, $2, $3, $4, COALESCE(sqlc.narg(effective_from)::date, current_date))
RETURNING id, tenant_id, hsn_sac, name, rate_bps, effective_from;

-- ─────────────────────────────────────────────── invoice_number_series

-- name: EnsureInvoiceSeries :exec
INSERT INTO invoice_number_series (tenant_id, fin_year, prefix, next_seq)
VALUES ($1, $2, COALESCE(sqlc.narg(prefix)::text, 'INV'), 1)
ON CONFLICT (tenant_id, fin_year) DO NOTHING;

-- name: AllocateInvoiceNumber :one
-- Run after EnsureInvoiceSeries, inside the invoice-creation transaction.
-- The UPDATE row-locks the series so concurrent allocations serialise;
-- a gap only appears if the surrounding transaction rolls back.
UPDATE invoice_number_series
SET next_seq = next_seq + 1
WHERE tenant_id = $1 AND fin_year = $2
RETURNING prefix, (next_seq - 1)::bigint AS seq;

-- name: EnsureCreditNoteSeries :exec
INSERT INTO credit_note_number_series (tenant_id, fin_year, prefix, next_seq)
VALUES ($1, $2, COALESCE(sqlc.narg(prefix)::text, 'CN'), 1)
ON CONFLICT (tenant_id, fin_year) DO NOTHING;

-- name: AllocateCreditNoteNumber :one
UPDATE credit_note_number_series
SET next_seq = next_seq + 1
WHERE tenant_id = $1 AND fin_year = $2
RETURNING prefix, (next_seq - 1)::bigint AS seq;

-- ──────────────────────────────────────────────────────────── invoices

-- name: CreateInvoice :one
INSERT INTO invoices (
    tenant_id, order_id, number, fin_year, supply_type, place_of_supply,
    buyer_snapshot, seller_snapshot, taxable_minor, cgst_minor, sgst_minor,
    igst_minor, cess_minor, round_off_minor, total_minor, pdf_key
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
        COALESCE(sqlc.narg(cess_minor)::bigint, 0),
        COALESCE(sqlc.narg(round_off_minor)::bigint, 0),
        $13, sqlc.narg(pdf_key)::text)
RETURNING id, tenant_id, order_id, number, fin_year, status, supply_type,
          place_of_supply, taxable_minor, cgst_minor, sgst_minor, igst_minor,
          round_off_minor, total_minor, issued_at, created_at;

-- name: CreateInvoiceLineItem :exec
INSERT INTO invoice_line_items (
    tenant_id, invoice_id, description, hsn_sac, qty, unit_minor, taxable_minor,
    rate_bps, cgst_minor, sgst_minor, igst_minor, total_minor
)
VALUES ($1, $2, $3, sqlc.narg(hsn_sac)::text, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: GetInvoiceByOrder :one
SELECT id, number, fin_year, status, supply_type, place_of_supply,
       taxable_minor, cgst_minor, sgst_minor, igst_minor, round_off_minor,
       total_minor, issued_at, pdf_key
FROM invoices WHERE order_id = $1;

-- name: GetInvoiceByID :one
SELECT id, tenant_id, order_id, number, fin_year, status, supply_type,
       place_of_supply, buyer_snapshot, seller_snapshot, taxable_minor,
       cgst_minor, sgst_minor, igst_minor, round_off_minor, total_minor,
       issued_at, pdf_key
FROM invoices WHERE id = $1;

-- name: ListInvoiceLineItems :many
SELECT id, description, hsn_sac, qty, unit_minor, taxable_minor, rate_bps,
       cgst_minor, sgst_minor, igst_minor, total_minor
FROM invoice_line_items WHERE invoice_id = $1 ORDER BY created_at;

-- name: CancelInvoice :exec
UPDATE invoices SET status = 'cancelled' WHERE id = $1;

-- name: ListTenantInvoices :many
SELECT id, number, order_id, status, total_minor, issued_at
FROM invoices WHERE tenant_id = $1
ORDER BY issued_at DESC LIMIT $2 OFFSET $3;

-- name: ListInvoicesForUser :many
SELECT i.id, i.number, i.order_id, i.status, i.supply_type, i.taxable_minor,
       i.cgst_minor, i.sgst_minor, i.igst_minor, i.total_minor, i.issued_at
FROM invoices i
JOIN orders o ON o.id = i.order_id
WHERE i.tenant_id = $1 AND o.user_id = $2
ORDER BY i.issued_at DESC LIMIT $3 OFFSET $4;

-- name: GetInvoiceForUser :one
SELECT i.id, i.tenant_id, i.order_id, i.number, i.fin_year, i.status,
       i.supply_type, i.place_of_supply, i.buyer_snapshot, i.seller_snapshot,
       i.taxable_minor, i.cgst_minor, i.sgst_minor, i.igst_minor,
       i.round_off_minor, i.total_minor, i.issued_at
FROM invoices i
JOIN orders o ON o.id = i.order_id
WHERE i.id = $1 AND o.user_id = $2;

-- name: SetInvoicePDFKey :exec
UPDATE invoices SET pdf_key = $2 WHERE id = $1;

-- ─────────────────────────────────────────────────────────── credit_notes

-- name: CreateCreditNote :one
INSERT INTO credit_notes (
    tenant_id, invoice_id, refund_id, number, fin_year, reason, taxable_minor,
    cgst_minor, sgst_minor, igst_minor, round_off_minor, total_minor, pdf_key
)
VALUES ($1, $2, sqlc.narg(refund_id)::uuid, $3, $4, sqlc.narg(reason)::text,
        $5, $6, $7, $8, COALESCE(sqlc.narg(round_off_minor)::bigint, 0), $9,
        sqlc.narg(pdf_key)::text)
RETURNING id, tenant_id, invoice_id, refund_id, number, fin_year, taxable_minor,
          cgst_minor, sgst_minor, igst_minor, total_minor, issued_at;

-- name: CreateCreditNoteLineItem :exec
INSERT INTO credit_note_line_items (
    tenant_id, credit_note_id, description, hsn_sac, qty, unit_minor,
    taxable_minor, rate_bps, cgst_minor, sgst_minor, igst_minor, total_minor
)
VALUES ($1, $2, $3, sqlc.narg(hsn_sac)::text, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: GetCreditNoteByRefund :one
SELECT id, invoice_id, number, total_minor, issued_at
FROM credit_notes WHERE refund_id = $1;
