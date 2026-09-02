-- 0063_wallet_payouts.sql
-- Store credit (referral rewards, refund-to-wallet) and outbound payouts
-- (affiliate, instructor revenue share, gateway refunds-as-payout).

-- ─────────────────────────────────────────────────────────────────── wallets
CREATE TABLE wallets (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    balance_minor bigint NOT NULL DEFAULT 0 CHECK (balance_minor >= 0),
    currency      text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id)
);
CREATE INDEX idx_wallets_user ON wallets (user_id);
SELECT apply_tenant_table('wallets');

CREATE TABLE wallet_transactions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    wallet_id           uuid NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount_minor        bigint NOT NULL,          -- signed
    kind                wallet_txn_kind NOT NULL,
    ref_type            text,
    ref_id              uuid,
    balance_after_minor bigint NOT NULL CHECK (balance_after_minor >= 0),
    note                text,
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_wallet_transactions_wallet ON wallet_transactions (wallet_id, created_at DESC);
CREATE INDEX idx_wallet_transactions_tenant ON wallet_transactions (tenant_id);
CREATE INDEX idx_wallet_transactions_ref ON wallet_transactions (ref_type, ref_id);
SELECT apply_tenant_rls('wallet_transactions');
SELECT apply_app_grants('wallet_transactions');

-- ─────────────────────────────────────────────────────────────────── payouts
CREATE TABLE payouts (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    payee_user_id    uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    kind             payout_kind NOT NULL,
    amount_minor     bigint NOT NULL CHECK (amount_minor > 0),
    tds_minor        bigint NOT NULL DEFAULT 0 CHECK (tds_minor >= 0),
    net_minor        bigint NOT NULL CHECK (net_minor >= 0),
    currency         text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    status           payout_status NOT NULL DEFAULT 'pending',
    method           text,
    gateway_payout_id text,
    note             text,
    requested_at     timestamptz NOT NULL DEFAULT now(),
    processed_at     timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_payouts_tenant_status ON payouts (tenant_id, status);
CREATE INDEX idx_payouts_payee ON payouts (payee_user_id, requested_at DESC);
SELECT apply_tenant_table('payouts');

CREATE TABLE payout_items (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    payout_id    uuid NOT NULL REFERENCES payouts(id) ON DELETE CASCADE,
    ref_type     text NOT NULL,          -- order_item | refund | referral_event
    ref_id       uuid NOT NULL,
    amount_minor bigint NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_payout_items_payout ON payout_items (payout_id);
CREATE INDEX idx_payout_items_tenant ON payout_items (tenant_id);
CREATE INDEX idx_payout_items_ref ON payout_items (ref_type, ref_id);
SELECT apply_tenant_rls('payout_items');
SELECT apply_app_grants('payout_items');
