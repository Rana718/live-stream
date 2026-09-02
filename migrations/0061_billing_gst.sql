-- 0061_billing_gst.sql
-- India GST invoicing. Invoices are IMMUTABLE (no updated_at) with gapless
-- per-tenant per-FY numbering; corrections happen via credit_notes.
--
-- ROUNDING (enforced in internal/billing): CGST/SGST/IGST rounded to paise
-- per line, summed, then a single invoice-level round_off_minor to the
-- nearest rupee.

-- ─────────────────────────────────────────────────────────────── tax_rates
CREATE TABLE tax_rates (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid REFERENCES tenants(id) ON DELETE CASCADE,  -- null = platform default
    hsn_sac        text NOT NULL,
    name           text NOT NULL,
    rate_bps       integer NOT NULL CHECK (rate_bps BETWEEN 0 AND 10000),
    effective_from date NOT NULL DEFAULT current_date,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_tax_rates_lookup ON tax_rates (hsn_sac, effective_from DESC);
CREATE INDEX idx_tax_rates_tenant ON tax_rates (tenant_id);

ALTER TABLE tax_rates ENABLE ROW LEVEL SECURITY;
ALTER TABLE tax_rates FORCE ROW LEVEL SECURITY;
CREATE POLICY tax_rates_read ON tax_rates FOR SELECT
    USING (tenant_id IS NULL OR tenant_id = current_tenant_id() OR is_super_admin());
CREATE POLICY tax_rates_tenant_write ON tax_rates
    USING (tenant_id = current_tenant_id() OR is_super_admin())
    WITH CHECK (tenant_id = current_tenant_id() OR is_super_admin());
SELECT apply_updated_at('tax_rates');
SELECT apply_app_grants('tax_rates');

-- ──────────────────────────────────────────────────── invoice_number_series
CREATE TABLE invoice_number_series (
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    fin_year  text NOT NULL,                 -- "2026-27"
    prefix    text NOT NULL DEFAULT 'INV',
    next_seq  bigint NOT NULL DEFAULT 1 CHECK (next_seq >= 1),
    PRIMARY KEY (tenant_id, fin_year)
);
SELECT apply_tenant_rls('invoice_number_series');
SELECT apply_app_grants('invoice_number_series');

-- ──────────────────────────────────────────────────────────────── invoices
CREATE TABLE invoices (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    order_id        uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    number          text NOT NULL,
    fin_year        text NOT NULL,
    status          invoice_status NOT NULL DEFAULT 'issued',
    supply_type     gst_supply_type NOT NULL,
    place_of_supply text NOT NULL,
    buyer_snapshot  jsonb NOT NULL,
    seller_snapshot jsonb NOT NULL,
    currency        text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    taxable_minor   bigint NOT NULL CHECK (taxable_minor >= 0),
    cgst_minor      bigint NOT NULL DEFAULT 0 CHECK (cgst_minor >= 0),
    sgst_minor      bigint NOT NULL DEFAULT 0 CHECK (sgst_minor >= 0),
    igst_minor      bigint NOT NULL DEFAULT 0 CHECK (igst_minor >= 0),
    cess_minor      bigint NOT NULL DEFAULT 0 CHECK (cess_minor >= 0),
    round_off_minor bigint NOT NULL DEFAULT 0 CHECK (round_off_minor BETWEEN -100 AND 100),
    total_minor     bigint NOT NULL CHECK (total_minor >= 0),
    irn             text,
    irn_qr          text,
    irn_status      text,
    irn_acked_at    timestamptz,
    issued_at       timestamptz NOT NULL DEFAULT now(),
    pdf_key         text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, number)
);
CREATE INDEX idx_invoices_order ON invoices (order_id);
CREATE INDEX idx_invoices_tenant_issued ON invoices (tenant_id, issued_at DESC);
SELECT apply_tenant_rls('invoices');
SELECT apply_app_grants('invoices');

CREATE TABLE invoice_line_items (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    invoice_id    uuid NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    description   text NOT NULL,
    hsn_sac       text,
    qty           integer NOT NULL DEFAULT 1 CHECK (qty >= 1),
    unit_minor    bigint NOT NULL CHECK (unit_minor >= 0),
    taxable_minor bigint NOT NULL CHECK (taxable_minor >= 0),
    rate_bps      integer NOT NULL DEFAULT 0,
    cgst_minor    bigint NOT NULL DEFAULT 0,
    sgst_minor    bigint NOT NULL DEFAULT 0,
    igst_minor    bigint NOT NULL DEFAULT 0,
    total_minor   bigint NOT NULL CHECK (total_minor >= 0),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_invoice_line_items_invoice ON invoice_line_items (invoice_id);
CREATE INDEX idx_invoice_line_items_tenant ON invoice_line_items (tenant_id);
SELECT apply_tenant_rls('invoice_line_items');
SELECT apply_app_grants('invoice_line_items');

-- ────────────────────────────────────────────────── credit_note_number_series
CREATE TABLE credit_note_number_series (
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    fin_year  text NOT NULL,
    prefix    text NOT NULL DEFAULT 'CN',
    next_seq  bigint NOT NULL DEFAULT 1 CHECK (next_seq >= 1),
    PRIMARY KEY (tenant_id, fin_year)
);
SELECT apply_tenant_rls('credit_note_number_series');
SELECT apply_app_grants('credit_note_number_series');

-- ──────────────────────────────────────────────────────────────── credit_notes
CREATE TABLE credit_notes (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    invoice_id      uuid NOT NULL REFERENCES invoices(id) ON DELETE RESTRICT,
    refund_id       uuid REFERENCES refunds(id) ON DELETE SET NULL,
    number          text NOT NULL,
    fin_year        text NOT NULL,
    reason          text,
    currency        text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    taxable_minor   bigint NOT NULL CHECK (taxable_minor >= 0),
    cgst_minor      bigint NOT NULL DEFAULT 0,
    sgst_minor      bigint NOT NULL DEFAULT 0,
    igst_minor      bigint NOT NULL DEFAULT 0,
    round_off_minor bigint NOT NULL DEFAULT 0 CHECK (round_off_minor BETWEEN -100 AND 100),
    total_minor     bigint NOT NULL CHECK (total_minor >= 0),
    issued_at       timestamptz NOT NULL DEFAULT now(),
    pdf_key         text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, number)
);
CREATE INDEX idx_credit_notes_invoice ON credit_notes (invoice_id);
CREATE INDEX idx_credit_notes_refund ON credit_notes (refund_id);
SELECT apply_tenant_rls('credit_notes');
SELECT apply_app_grants('credit_notes');

CREATE TABLE credit_note_line_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    credit_note_id uuid NOT NULL REFERENCES credit_notes(id) ON DELETE CASCADE,
    description    text NOT NULL,
    hsn_sac        text,
    qty            integer NOT NULL DEFAULT 1,
    unit_minor     bigint NOT NULL,
    taxable_minor  bigint NOT NULL,
    rate_bps       integer NOT NULL DEFAULT 0,
    cgst_minor     bigint NOT NULL DEFAULT 0,
    sgst_minor     bigint NOT NULL DEFAULT 0,
    igst_minor     bigint NOT NULL DEFAULT 0,
    total_minor    bigint NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_credit_note_line_items_cn ON credit_note_line_items (credit_note_id);
CREATE INDEX idx_credit_note_line_items_tenant ON credit_note_line_items (tenant_id);
SELECT apply_tenant_rls('credit_note_line_items');
SELECT apply_app_grants('credit_note_line_items');
