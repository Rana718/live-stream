-- 0020_tenancy.sql
-- The multi-tenant control plane. `tenants` IS the tenant (no tenant_id);
-- everything else here is tenant-scoped and gets the standard RLS via
-- apply_tenant_table(). tenant_users is the SOLE source of a user's role.

-- ───────────────────────────────────────────────────────────── tenants
CREATE TABLE tenants (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_code           citext NOT NULL UNIQUE,
    name               text NOT NULL,
    slug               citext NOT NULL UNIQUE,
    parent_tenant_id   uuid REFERENCES tenants(id) ON DELETE SET NULL,
    status             tenant_status NOT NULL DEFAULT 'trial',
    plan               tenant_plan NOT NULL DEFAULT 'starter',
    logo_url           text,
    theme              jsonb NOT NULL DEFAULT '{}'::jsonb,
    legal_name         text,
    gstin              text,
    pan                text,
    registered_address jsonb NOT NULL DEFAULT '{}'::jsonb,
    place_of_supply    text,               -- 2-digit GST state code
    billing_email      citext,
    razorpay_account_id text,
    timezone           text NOT NULL DEFAULT 'Asia/Kolkata',
    locale             text NOT NULL DEFAULT 'en',
    trial_ends_at      timestamptz,
    owner_user_id      uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz,
    CONSTRAINT tenants_no_self_parent CHECK (parent_tenant_id IS NULL OR parent_tenant_id <> id)
);
CREATE INDEX idx_tenants_parent ON tenants (parent_tenant_id);
CREATE INDEX idx_tenants_status ON tenants (status);

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
CREATE POLICY tenants_super_admin ON tenants
    USING (is_super_admin()) WITH CHECK (is_super_admin());
CREATE POLICY tenants_self_read ON tenants
    FOR SELECT USING (id = current_tenant_id() OR is_super_admin());
CREATE POLICY tenants_self_write ON tenants
    FOR UPDATE USING (id = current_tenant_id()) WITH CHECK (id = current_tenant_id());
CREATE POLICY tenants_public_lookup ON tenants
    FOR SELECT USING (is_public_lookup());
SELECT apply_updated_at('tenants');
SELECT apply_app_grants('tenants');

-- Deferred FK from 0010.
ALTER TABLE refresh_tokens
    ADD CONSTRAINT refresh_tokens_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
CREATE INDEX idx_refresh_tokens_tenant ON refresh_tokens (tenant_id);

-- ──────────────────────────────────────────────────────── tenant_domains
CREATE TABLE tenant_domains (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain      citext NOT NULL UNIQUE,
    is_primary  boolean NOT NULL DEFAULT false,
    verified_at timestamptz,
    ssl_status  text NOT NULL DEFAULT 'pending' CHECK (ssl_status IN ('pending','active','failed')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_tenant_domains_tenant ON tenant_domains (tenant_id);
SELECT apply_tenant_table('tenant_domains');
-- CustomDomain middleware resolves domain -> tenant before auth.
CREATE POLICY tenant_domains_public_lookup ON tenant_domains
    FOR SELECT USING (is_public_lookup());

-- ─────────────────────────────────────────────────────── tenant_settings
CREATE TABLE tenant_settings (
    tenant_id           uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    features            jsonb NOT NULL DEFAULT '{}'::jsonb,
    theme               jsonb NOT NULL DEFAULT '{}'::jsonb,
    payment_config      jsonb NOT NULL DEFAULT '{}'::jsonb,
    notification_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
SELECT apply_tenant_table('tenant_settings');

-- ─────────────────────────────────────────────────────────── tenant_users
CREATE TABLE tenant_users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       tenant_role NOT NULL,
    status     membership_status NOT NULL DEFAULT 'active',
    invited_by uuid REFERENCES users(id) ON DELETE SET NULL,
    invited_at timestamptz,
    joined_at  timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, user_id)
);
CREATE INDEX idx_tenant_users_user ON tenant_users (user_id);
CREATE INDEX idx_tenant_users_tenant_role ON tenant_users (tenant_id, role);
SELECT apply_tenant_table('tenant_users');
-- Login lists a user's memberships before a tenant is chosen.
CREATE POLICY tenant_users_self_read ON tenant_users
    FOR SELECT USING (user_id = current_app_user() OR is_super_admin());

-- ─────────────────────────────────────────────────────────── user_profiles
CREATE TABLE user_profiles (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    class_level          text,
    board                text,
    exam_goal            text,
    onboarding_completed boolean NOT NULL DEFAULT false,
    guardian_name        text,
    guardian_phone       citext,
    address              jsonb NOT NULL DEFAULT '{}'::jsonb,
    meta                 jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id)
);
CREATE INDEX idx_user_profiles_user ON user_profiles (user_id);
SELECT apply_tenant_table('user_profiles');

-- ────────────────────────────────────────────────── platform_subscriptions
-- Our billing OF a tenant (distinct from a tenant's student subscriptions).
CREATE TABLE platform_subscriptions (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    plan                    tenant_plan NOT NULL,
    status                  subscription_status NOT NULL DEFAULT 'trialing',
    amount_minor            bigint NOT NULL DEFAULT 0 CHECK (amount_minor >= 0),
    currency                text NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    current_period_start    timestamptz,
    current_period_end      timestamptz,
    trial_ends_at           timestamptz,
    gateway_subscription_id text,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_platform_subs_tenant ON platform_subscriptions (tenant_id);
SELECT apply_tenant_table('platform_subscriptions');

-- ──────────────────────────────────────────────────────────── app_builds
CREATE TABLE app_builds (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    platform     build_platform NOT NULL,
    status       build_status NOT NULL DEFAULT 'queued',
    package_id   text,
    version_code integer,
    version_name text,
    build_url    text,
    store_url    text,
    error_log    text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);
CREATE INDEX idx_app_builds_tenant ON app_builds (tenant_id, status);
SELECT apply_tenant_table('app_builds');

-- ──────────────────────────────────────────────────────────────── leads
-- Marketing-site capture. tenant_id is null until converted. Public form
-- can INSERT; only platform staff can read/triage.
CREATE TABLE leads (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid REFERENCES tenants(id) ON DELETE SET NULL,
    name           text,
    phone          citext,
    email          citext,
    institute_name text,
    city           text,
    students_count integer,
    source         text,
    utm            jsonb NOT NULL DEFAULT '{}'::jsonb,
    status         lead_status NOT NULL DEFAULT 'new',
    assigned_to    uuid REFERENCES users(id) ON DELETE SET NULL,
    notes          text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_leads_status ON leads (status, created_at DESC);

ALTER TABLE leads ENABLE ROW LEVEL SECURITY;
ALTER TABLE leads FORCE ROW LEVEL SECURITY;
CREATE POLICY leads_super_admin ON leads
    USING (is_super_admin()) WITH CHECK (is_super_admin());
CREATE POLICY leads_public_insert ON leads
    FOR INSERT WITH CHECK (true);
SELECT apply_updated_at('leads');
SELECT apply_app_grants('leads');
