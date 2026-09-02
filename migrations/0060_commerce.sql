-- 0060_commerce.sql
-- The commerce core. Every amount is bigint minor units (paise) + an INR
-- currency guard. Access is granted by `entitlements` (decoupled from
-- payment); `orders` is the single order table for all four product kinds.

-- ─────────────────────────────────────────────────────────── course_bundles
CREATE TABLE course_bundles (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    title         text NOT NULL,
    description   text,
    cover_url     text,
    is_active     boolean NOT NULL DEFAULT true,
    display_order integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);
CREATE INDEX idx_course_bundles_tenant ON course_bundles (tenant_id, is_active);
SELECT apply_tenant_table('course_bundles');

-- ─────────────────────────────────────────────────────── subscription_plans
CREATE TABLE subscription_plans (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    name          text NOT NULL,
    slug          citext NOT NULL,
    description   text,
    interval      subscription_interval NOT NULL DEFAULT 'monthly',
    interval_days integer NOT NULL DEFAULT 30 CHECK (interval_days > 0),
    trial_days    integer NOT NULL DEFAULT 0 CHECK (trial_days >= 0),
    features      jsonb NOT NULL DEFAULT '[]'::jsonb,
    hsn_sac       text,
    tax_rate_bps  integer NOT NULL DEFAULT 0 CHECK (tax_rate_bps BETWEEN 0 AND 10000),
    is_active     boolean NOT NULL DEFAULT true,
    display_order integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz,
    UNIQUE (tenant_id, slug)
);
CREATE INDEX idx_subscription_plans_tenant ON subscription_plans (tenant_id, is_active);
SELECT apply_tenant_table('subscription_plans');

-- ─────────────────────────────────────────────────────────────── fee_plans
CREATE TABLE fee_plans (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id          uuid REFERENCES courses(id) ON DELETE SET NULL,
    batch_id           uuid REFERENCES batches(id) ON DELETE SET NULL,
    name               text NOT NULL,
    total_minor        bigint NOT NULL CHECK (total_minor >= 0),
    currency           text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    installments_count integer NOT NULL DEFAULT 1 CHECK (installments_count >= 1),
    gap_days           integer NOT NULL DEFAULT 30 CHECK (gap_days >= 0),
    late_fee_minor     bigint NOT NULL DEFAULT 0 CHECK (late_fee_minor >= 0),
    hsn_sac            text,
    tax_rate_bps       integer NOT NULL DEFAULT 0 CHECK (tax_rate_bps BETWEEN 0 AND 10000),
    is_active          boolean NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz
);
CREATE INDEX idx_fee_plans_tenant ON fee_plans (tenant_id);
CREATE INDEX idx_fee_plans_course ON fee_plans (course_id);
CREATE INDEX idx_fee_plans_batch ON fee_plans (batch_id);
SELECT apply_tenant_table('fee_plans');

-- ───────────────────────────────────────────────────────────────── products
-- Typed registry. Exactly one of the four *_id columns is non-null,
-- matching `kind`. Coupons / prices / order_items / entitlements all point
-- here so the four checkout flows share one code path.
CREATE TABLE products (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    kind         product_kind NOT NULL,
    course_id    uuid REFERENCES courses(id) ON DELETE CASCADE,
    bundle_id    uuid REFERENCES course_bundles(id) ON DELETE CASCADE,
    plan_id      uuid REFERENCES subscription_plans(id) ON DELETE CASCADE,
    fee_plan_id  uuid REFERENCES fee_plans(id) ON DELETE CASCADE,
    hsn_sac      text,
    tax_rate_bps integer NOT NULL DEFAULT 0 CHECK (tax_rate_bps BETWEEN 0 AND 10000),
    is_active    boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,
    CONSTRAINT products_one_ref CHECK (
        num_nonnulls(course_id, bundle_id, plan_id, fee_plan_id) = 1
    ),
    CONSTRAINT products_kind_matches_ref CHECK (
        (kind = 'course'   AND course_id   IS NOT NULL) OR
        (kind = 'bundle'   AND bundle_id   IS NOT NULL) OR
        (kind = 'plan'     AND plan_id     IS NOT NULL) OR
        (kind = 'fee_plan' AND fee_plan_id IS NOT NULL)
    )
);
CREATE UNIQUE INDEX uq_products_course ON products (course_id)   WHERE course_id   IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX uq_products_bundle ON products (bundle_id)   WHERE bundle_id   IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX uq_products_plan   ON products (plan_id)     WHERE plan_id     IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX uq_products_fee    ON products (fee_plan_id) WHERE fee_plan_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_products_tenant_kind ON products (tenant_id, kind) WHERE is_active AND deleted_at IS NULL;
SELECT apply_tenant_table('products');

-- ─────────────────────────────────────────────────────────────── bundle_items
CREATE TABLE bundle_items (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    bundle_product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    item_product_id   uuid NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    position          integer NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bundle_product_id, item_product_id),
    CONSTRAINT bundle_items_no_self CHECK (bundle_product_id <> item_product_id)
);
CREATE INDEX idx_bundle_items_item ON bundle_items (item_product_id);
CREATE INDEX idx_bundle_items_tenant ON bundle_items (tenant_id);
SELECT apply_tenant_table('bundle_items');

-- ────────────────────────────────────────────────────────────────── prices
CREATE TABLE prices (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    product_id      uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    amount_minor    bigint NOT NULL CHECK (amount_minor >= 0),
    compare_at_minor bigint CHECK (compare_at_minor IS NULL OR compare_at_minor >= 0),
    currency        text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    valid_from      timestamptz NOT NULL DEFAULT now(),
    valid_to        timestamptz,
    is_active       boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);
CREATE UNIQUE INDEX uq_prices_active ON prices (product_id) WHERE is_active AND deleted_at IS NULL;
CREATE INDEX idx_prices_tenant ON prices (tenant_id);
SELECT apply_tenant_table('prices');

-- ────────────────────────────────────────────────────────────────── coupons
CREATE TABLE coupons (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    code               citext NOT NULL,
    type               coupon_type NOT NULL,
    percent_bps        integer CHECK (percent_bps IS NULL OR percent_bps BETWEEN 1 AND 10000),
    value_minor        bigint CHECK (value_minor IS NULL OR value_minor > 0),
    max_discount_minor bigint CHECK (max_discount_minor IS NULL OR max_discount_minor > 0),
    min_order_minor    bigint NOT NULL DEFAULT 0 CHECK (min_order_minor >= 0),
    applies_to         coupon_scope NOT NULL DEFAULT 'all',
    starts_at          timestamptz NOT NULL DEFAULT now(),
    ends_at            timestamptz,
    usage_limit        integer,
    per_user_limit     integer NOT NULL DEFAULT 1 CHECK (per_user_limit >= 1),
    used_count         integer NOT NULL DEFAULT 0,
    is_active          boolean NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code),
    CONSTRAINT coupons_value_matches_type CHECK (
        (type = 'percent' AND percent_bps IS NOT NULL AND value_minor IS NULL) OR
        (type = 'flat'    AND value_minor IS NOT NULL AND percent_bps IS NULL)
    )
);
CREATE INDEX idx_coupons_tenant_active ON coupons (tenant_id, is_active);
SELECT apply_tenant_table('coupons');

CREATE TABLE coupon_products (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    coupon_id  uuid NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (coupon_id, product_id)
);
CREATE INDEX idx_coupon_products_product ON coupon_products (product_id);
CREATE INDEX idx_coupon_products_tenant ON coupon_products (tenant_id);
SELECT apply_tenant_table('coupon_products');

-- ─────────────────────────────────────────────────────────────────── orders
CREATE TABLE orders (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id            uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    code               text NOT NULL,
    status             order_status NOT NULL DEFAULT 'pending',
    subtotal_minor     bigint NOT NULL DEFAULT 0 CHECK (subtotal_minor >= 0),
    discount_minor     bigint NOT NULL DEFAULT 0 CHECK (discount_minor >= 0),
    tax_minor          bigint NOT NULL DEFAULT 0 CHECK (tax_minor >= 0),
    total_minor        bigint NOT NULL DEFAULT 0 CHECK (total_minor >= 0),
    currency           text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    coupon_id          uuid REFERENCES coupons(id) ON DELETE SET NULL,
    gateway            text,
    gateway_order_id   text,
    place_of_supply    text,
    notes              jsonb NOT NULL DEFAULT '{}'::jsonb,
    refund_deadline_at timestamptz,
    placed_at          timestamptz,
    paid_at            timestamptz,
    cancelled_at       timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code)
);
CREATE UNIQUE INDEX uq_orders_gateway_order_id ON orders (gateway_order_id)
    WHERE gateway_order_id IS NOT NULL AND gateway_order_id <> '';
CREATE INDEX idx_orders_tenant_user ON orders (tenant_id, user_id);
CREATE INDEX idx_orders_tenant_status_created ON orders (tenant_id, status, created_at DESC);
CREATE INDEX idx_orders_coupon ON orders (coupon_id);
SELECT apply_tenant_table('orders');

CREATE TABLE order_items (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    order_id          uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id        uuid NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    product_kind      product_kind NOT NULL,
    title             text NOT NULL,
    hsn_sac           text,
    unit_minor        bigint NOT NULL CHECK (unit_minor >= 0),
    qty               integer NOT NULL DEFAULT 1 CHECK (qty >= 1),
    line_subtotal_minor bigint NOT NULL CHECK (line_subtotal_minor >= 0),
    discount_minor    bigint NOT NULL DEFAULT 0 CHECK (discount_minor >= 0),
    taxable_minor     bigint NOT NULL DEFAULT 0 CHECK (taxable_minor >= 0),
    cgst_minor        bigint NOT NULL DEFAULT 0 CHECK (cgst_minor >= 0),
    sgst_minor        bigint NOT NULL DEFAULT 0 CHECK (sgst_minor >= 0),
    igst_minor        bigint NOT NULL DEFAULT 0 CHECK (igst_minor >= 0),
    total_minor       bigint NOT NULL CHECK (total_minor >= 0),
    grants_entitlement boolean NOT NULL DEFAULT true,
    entitlement_days  integer,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_order_items_order ON order_items (order_id);
CREATE INDEX idx_order_items_product ON order_items (product_id);
CREATE INDEX idx_order_items_tenant ON order_items (tenant_id);
SELECT apply_tenant_table('order_items');

-- ─────────────────────────────────────────────────────────────────── payments
-- One order, many attempts.
CREATE TABLE payments (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    order_id           uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    user_id            uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    gateway            text NOT NULL DEFAULT 'razorpay',
    gateway_order_id   text,
    gateway_payment_id text,
    method             payment_method,
    status             payment_status NOT NULL DEFAULT 'created',
    amount_minor       bigint NOT NULL CHECK (amount_minor >= 0),
    currency           text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    gateway_fee_minor  bigint NOT NULL DEFAULT 0 CHECK (gateway_fee_minor >= 0),
    signature          text,
    failure_reason     text,
    captured_at        timestamptz,
    failed_at          timestamptz,
    raw                jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_payments_gateway_payment_id ON payments (gateway_payment_id)
    WHERE gateway_payment_id IS NOT NULL AND gateway_payment_id <> '';
CREATE INDEX idx_payments_order ON payments (order_id);
CREATE INDEX idx_payments_tenant_status_created ON payments (tenant_id, status, created_at DESC);
CREATE INDEX idx_payments_gateway_order_id ON payments (gateway_order_id);
SELECT apply_tenant_table('payments');

CREATE TABLE payment_splits (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    payment_id         uuid NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    linked_account_id  text NOT NULL,
    amount_minor       bigint NOT NULL CHECK (amount_minor >= 0),
    on_hold            boolean NOT NULL DEFAULT false,
    gateway_transfer_id text,
    settled_at         timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_payment_splits_payment ON payment_splits (payment_id);
CREATE INDEX idx_payment_splits_tenant ON payment_splits (tenant_id);
SELECT apply_tenant_table('payment_splits');

-- ─────────────────────────────────────────────────────────────────── refunds
CREATE TABLE refunds (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    payment_id        uuid NOT NULL REFERENCES payments(id) ON DELETE RESTRICT,
    order_item_id     uuid REFERENCES order_items(id) ON DELETE SET NULL,
    amount_minor      bigint NOT NULL CHECK (amount_minor > 0),
    currency          text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    reason            refund_reason NOT NULL DEFAULT 'requested_by_customer',
    status            refund_status NOT NULL DEFAULT 'pending',
    gateway_refund_id text,
    notes             text,
    initiated_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    processed_at      timestamptz
);
CREATE UNIQUE INDEX uq_refunds_gateway_refund_id ON refunds (gateway_refund_id)
    WHERE gateway_refund_id IS NOT NULL AND gateway_refund_id <> '';
CREATE INDEX idx_refunds_payment ON refunds (payment_id);
CREATE INDEX idx_refunds_tenant_status ON refunds (tenant_id, status);
SELECT apply_tenant_table('refunds');

-- ─────────────────────────────────────────────────────────────── coupon_redemptions
CREATE TABLE coupon_redemptions (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    coupon_id        uuid NOT NULL REFERENCES coupons(id) ON DELETE RESTRICT,
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id         uuid REFERENCES orders(id) ON DELETE SET NULL,
    amount_off_minor bigint NOT NULL CHECK (amount_off_minor >= 0),
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_coupon_redemptions_coupon_user ON coupon_redemptions (coupon_id, user_id);
CREATE INDEX idx_coupon_redemptions_tenant ON coupon_redemptions (tenant_id);
SELECT apply_tenant_table('coupon_redemptions');

-- ────────────────────────────────────────────────────────────── entitlements
-- The single access-grant table. A content/course access check reads here.
CREATE TABLE entitlements (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id    uuid NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    product_kind  product_kind NOT NULL,
    source        entitlement_source NOT NULL,
    order_item_id uuid REFERENCES order_items(id) ON DELETE SET NULL,
    source_ref    uuid,                       -- subscription_id / grant_id / gift_id …
    granted_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz,
    revoked_at    timestamptz,
    revoke_reason text,
    created_by    uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_entitlements_lookup ON entitlements (tenant_id, user_id, product_id)
    WHERE revoked_at IS NULL;
CREATE INDEX idx_entitlements_product ON entitlements (product_id);
CREATE INDEX idx_entitlements_order_item ON entitlements (order_item_id);
SELECT apply_tenant_table('entitlements');

-- ────────────────────────────────────────────────────────────── enrollments
-- Thin per-course progress projection of a course entitlement.
CREATE TABLE enrollments (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id      uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    batch_id       uuid REFERENCES batches(id) ON DELETE SET NULL,
    entitlement_id uuid REFERENCES entitlements(id) ON DELETE SET NULL,
    status         enrollment_status NOT NULL DEFAULT 'active',
    progress_bps   integer NOT NULL DEFAULT 0 CHECK (progress_bps BETWEEN 0 AND 10000),
    started_at     timestamptz,
    completed_at   timestamptz,
    expires_at     timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, course_id)
);
CREATE INDEX idx_enrollments_course ON enrollments (course_id);
CREATE INDEX idx_enrollments_batch ON enrollments (batch_id);
CREATE INDEX idx_enrollments_user ON enrollments (user_id);
SELECT apply_tenant_table('enrollments');
