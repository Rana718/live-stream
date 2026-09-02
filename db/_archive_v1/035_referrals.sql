-- 035_referrals.sql
-- Per-tenant referral programme. Each user gets a unique short code; a new
-- signup that mentions a code credits the referrer. Reward semantics
-- (credit amount, expiry) are policy-driven so a tenant can configure
-- their own scheme without DDL.

CREATE TABLE IF NOT EXISTS user_referral_codes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- 8-char base32 code; uppercase + digits, no ambiguous chars (0/O,1/I).
    code        VARCHAR(16) NOT NULL UNIQUE,
    uses        INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ DEFAULT now(),
    UNIQUE(tenant_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_referral_codes_tenant_user
    ON user_referral_codes(tenant_id, user_id);

CREATE TABLE IF NOT EXISTS referral_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code          VARCHAR(16) NOT NULL,
    referrer_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    referred_user UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Status moves: signed_up → first_purchase → rewarded.
    -- We pay the reward only after the referred user makes a purchase,
    -- otherwise dummy signups would drain the rewards bucket.
    status        VARCHAR(20) NOT NULL DEFAULT 'signed_up',
    reward_paise  INTEGER NOT NULL DEFAULT 0,
    rewarded_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_referral_events_tenant_referrer
    ON referral_events(tenant_id, referrer_id);
CREATE INDEX IF NOT EXISTS idx_referral_events_status
    ON referral_events(status);

ALTER TABLE user_referral_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_referral_codes FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_referral_codes ON user_referral_codes;
CREATE POLICY tenant_isolation_referral_codes ON user_referral_codes
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
DROP POLICY IF EXISTS super_admin_referral_codes ON user_referral_codes;
CREATE POLICY super_admin_referral_codes ON user_referral_codes
    USING (is_super_admin()) WITH CHECK (is_super_admin());

ALTER TABLE referral_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE referral_events FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_referral_events ON referral_events;
CREATE POLICY tenant_isolation_referral_events ON referral_events
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
DROP POLICY IF EXISTS super_admin_referral_events ON referral_events;
CREATE POLICY super_admin_referral_events ON referral_events
    USING (is_super_admin()) WITH CHECK (is_super_admin());
