-- 0062_subscriptions_fees.sql
-- Recurring access (subscriptions) and installment billing (fee accounts).
-- Each subscription renewal and each installment payment creates its own
-- `orders` row; access always flows through `entitlements`.

-- ─────────────────────────────────────────────────────────────── subscriptions
CREATE TABLE subscriptions (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id                 uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id                 uuid NOT NULL REFERENCES subscription_plans(id) ON DELETE RESTRICT,
    status                  subscription_status NOT NULL DEFAULT 'trialing',
    current_period_start    timestamptz,
    current_period_end      timestamptz,
    trial_end               timestamptz,
    cancel_at_period_end    boolean NOT NULL DEFAULT false,
    cancelled_at            timestamptz,
    gateway_subscription_id text,
    origin_order_id         uuid REFERENCES orders(id) ON DELETE SET NULL,
    latest_order_id         uuid REFERENCES orders(id) ON DELETE SET NULL,
    entitlement_id          uuid REFERENCES entitlements(id) ON DELETE SET NULL,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_subscriptions_tenant_user ON subscriptions (tenant_id, user_id);
CREATE INDEX idx_subscriptions_status ON subscriptions (status);
CREATE INDEX idx_subscriptions_period_end ON subscriptions (current_period_end);
CREATE UNIQUE INDEX uq_subscriptions_gateway_id ON subscriptions (gateway_subscription_id)
    WHERE gateway_subscription_id IS NOT NULL AND gateway_subscription_id <> '';
SELECT apply_tenant_table('subscriptions');

-- ─────────────────────────────────────────────────────────────── fee_accounts
CREATE TABLE fee_accounts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fee_plan_id  uuid REFERENCES fee_plans(id) ON DELETE SET NULL,
    course_id    uuid REFERENCES courses(id) ON DELETE SET NULL,
    batch_id     uuid REFERENCES batches(id) ON DELETE SET NULL,
    total_minor  bigint NOT NULL CHECK (total_minor >= 0),
    paid_minor   bigint NOT NULL DEFAULT 0 CHECK (paid_minor >= 0),
    waived_minor bigint NOT NULL DEFAULT 0 CHECK (waived_minor >= 0),
    currency     text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    status       fee_account_status NOT NULL DEFAULT 'pending',
    due_on       date,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fee_accounts_tenant_user ON fee_accounts (tenant_id, user_id);
CREATE INDEX idx_fee_accounts_status ON fee_accounts (status);
CREATE INDEX idx_fee_accounts_course ON fee_accounts (course_id);
SELECT apply_tenant_table('fee_accounts');

-- ─────────────────────────────────────────────────────────────── fee_installments
CREATE TABLE fee_installments (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    fee_account_id uuid NOT NULL REFERENCES fee_accounts(id) ON DELETE CASCADE,
    seq            integer NOT NULL CHECK (seq >= 1),
    amount_minor   bigint NOT NULL CHECK (amount_minor >= 0),
    due_on         date,
    status         installment_status NOT NULL DEFAULT 'pending',
    paid_at        timestamptz,
    order_id       uuid REFERENCES orders(id) ON DELETE SET NULL,
    waived_reason  text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (fee_account_id, seq)
);
CREATE INDEX idx_fee_installments_tenant ON fee_installments (tenant_id);
CREATE INDEX idx_fee_installments_due ON fee_installments (due_on) WHERE status = 'pending';
CREATE INDEX idx_fee_installments_order ON fee_installments (order_id);
SELECT apply_tenant_table('fee_installments');
