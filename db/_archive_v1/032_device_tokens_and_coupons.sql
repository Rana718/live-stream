-- 032_device_tokens_and_coupons.sql
-- Two unrelated additions bundled because they're both small and both
-- depend only on the multi-tenant retrofit being in place:
--
--   1. device_tokens — FCM push targets, one row per device per user.
--   2. coupons       — discount codes a tenant can issue.

-- 1. Device tokens.
CREATE TABLE IF NOT EXISTS device_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token           TEXT NOT NULL,
    platform        VARCHAR(10) NOT NULL,            -- android | ios | web
    -- Lets us evict tokens that haven't checked in for >30 days so we don't
    -- keep paying FCM to push to dead installs.
    last_seen_at    TIMESTAMPTZ DEFAULT now(),
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_device_tokens_unique_token ON device_tokens(token);
CREATE INDEX IF NOT EXISTS idx_device_tokens_user ON device_tokens(tenant_id, user_id);

ALTER TABLE device_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE device_tokens FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_device_tokens ON device_tokens;
CREATE POLICY tenant_isolation_device_tokens ON device_tokens
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

DROP POLICY IF EXISTS super_admin_device_tokens ON device_tokens;
CREATE POLICY super_admin_device_tokens ON device_tokens
    USING (is_super_admin())
    WITH CHECK (is_super_admin());

-- 2. Coupons.
--
-- discount_type:
--   'percent' — percentage off, capped at max_discount when set
--   'flat'    — fixed paise amount off
--
-- scope:
--   'all'        — applies to every course / fee / subscription in tenant
--   'course'     — only the course IDs in coupon_courses
--   'subscription' — only when a subscription plan is checked out
CREATE TABLE IF NOT EXISTS coupons (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code            VARCHAR(40) NOT NULL,
    discount_type   VARCHAR(10) NOT NULL,            -- percent|flat
    discount_value  INTEGER NOT NULL,                -- 1-100 for percent, paise for flat
    max_discount    INTEGER,                         -- paise cap for percent
    scope           VARCHAR(20) NOT NULL DEFAULT 'all',
    min_amount      INTEGER NOT NULL DEFAULT 0,      -- paise
    starts_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    ends_at         TIMESTAMPTZ,
    usage_limit     INTEGER,                         -- null = unlimited
    used_count      INTEGER NOT NULL DEFAULT 0,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT now(),
    UNIQUE(tenant_id, code)
);

CREATE INDEX IF NOT EXISTS idx_coupons_tenant_active ON coupons(tenant_id, is_active);

CREATE TABLE IF NOT EXISTS coupon_courses (
    coupon_id       UUID NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    course_id       UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    PRIMARY KEY (coupon_id, course_id)
);

-- Track each redemption — used to enforce per-user limits and for analytics.
CREATE TABLE IF NOT EXISTS coupon_redemptions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    coupon_id       UUID NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    payment_id      UUID REFERENCES payments(id) ON DELETE SET NULL,
    amount_off      INTEGER NOT NULL,                -- paise that got applied
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_coupon_redemptions_user
    ON coupon_redemptions(coupon_id, user_id);

ALTER TABLE coupons ENABLE ROW LEVEL SECURITY;
ALTER TABLE coupons FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_coupons ON coupons;
CREATE POLICY tenant_isolation_coupons ON coupons
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
DROP POLICY IF EXISTS super_admin_coupons ON coupons;
CREATE POLICY super_admin_coupons ON coupons
    USING (is_super_admin()) WITH CHECK (is_super_admin());

ALTER TABLE coupon_redemptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE coupon_redemptions FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_coupon_redemptions ON coupon_redemptions;
CREATE POLICY tenant_isolation_coupon_redemptions ON coupon_redemptions
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
DROP POLICY IF EXISTS super_admin_coupon_redemptions ON coupon_redemptions;
CREATE POLICY super_admin_coupon_redemptions ON coupon_redemptions
    USING (is_super_admin()) WITH CHECK (is_super_admin());
