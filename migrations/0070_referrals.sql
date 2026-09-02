-- 0070_referrals.sql
-- Per-user referral codes; a qualifying purchase by a referred user credits
-- the referrer's wallet exactly once (idempotent on qualifying_order_id).

CREATE TABLE referral_codes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code       citext NOT NULL UNIQUE,
    uses       integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id)
);
CREATE INDEX idx_referral_codes_user ON referral_codes (user_id);
SELECT apply_tenant_table('referral_codes');

CREATE TABLE referral_events (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    code                  citext NOT NULL,
    referrer_user_id      uuid REFERENCES users(id) ON DELETE SET NULL,
    referred_user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status                referral_status NOT NULL DEFAULT 'pending',
    reward_minor          bigint NOT NULL DEFAULT 0 CHECK (reward_minor >= 0),
    qualifying_order_id   uuid REFERENCES orders(id) ON DELETE SET NULL,
    wallet_transaction_id uuid REFERENCES wallet_transactions(id) ON DELETE SET NULL,
    rewarded_at           timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (referred_user_id)
);
CREATE INDEX idx_referral_events_tenant_referrer ON referral_events (tenant_id, referrer_user_id);
CREATE INDEX idx_referral_events_status ON referral_events (status);
CREATE UNIQUE INDEX uq_referral_events_qualifying_order ON referral_events (qualifying_order_id)
    WHERE qualifying_order_id IS NOT NULL;
SELECT apply_tenant_table('referral_events');
