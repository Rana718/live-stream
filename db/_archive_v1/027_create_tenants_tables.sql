-- 027_create_tenants_tables.sql
-- Phase 1 of multi-tenant retrofit. Establishes the tenants table and all
-- supporting metadata tables (tenant_users, tenant_features, app_builds,
-- platform_subscriptions, leads). The actual `tenant_id` columns on the
-- existing 28 business tables are added in migration 028.
--
-- Why split across multiple migrations: keeping the new tables isolated lets
-- us seed a default tenant before backfilling tenant_id, and lets RLS policies
-- in 029 reference a guaranteed-existing schema.

CREATE TABLE IF NOT EXISTS tenants (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_code            VARCHAR(20) UNIQUE NOT NULL,
    name                VARCHAR(200) NOT NULL,
    slug                VARCHAR(100) UNIQUE NOT NULL,
    custom_domain       VARCHAR(200) UNIQUE,
    logo_url            TEXT,
    theme               JSONB NOT NULL DEFAULT '{}'::jsonb,
    app_config          JSONB NOT NULL DEFAULT '{}'::jsonb,
    plan                VARCHAR(20) NOT NULL DEFAULT 'starter',
    status              VARCHAR(20) NOT NULL DEFAULT 'active',
    trial_ends_at       TIMESTAMPTZ,
    owner_user_id       UUID,
    razorpay_account_id VARCHAR(100),
    created_at          TIMESTAMPTZ DEFAULT now(),
    updated_at          TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tenants_org_code ON tenants(org_code);
CREATE INDEX IF NOT EXISTS idx_tenants_slug     ON tenants(slug);
CREATE INDEX IF NOT EXISTS idx_tenants_status   ON tenants(status);

-- Per-tenant per-role membership. Replaces the simple users.role column
-- conceptually (we keep users.role as a default fallback for the user's
-- "primary" tenant, but tenant_users is the source of truth when a user
-- belongs to multiple orgs).
CREATE TABLE IF NOT EXISTS tenant_users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        VARCHAR(20) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    invited_by  UUID,
    joined_at   TIMESTAMPTZ DEFAULT now(),
    UNIQUE(tenant_id, user_id, role)
);

CREATE INDEX IF NOT EXISTS idx_tenant_users_tenant ON tenant_users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_users_user   ON tenant_users(user_id);

-- Per-tenant feature flags. Plan-derived defaults plus admin overrides.
CREATE TABLE IF NOT EXISTS tenant_features (
    tenant_id   UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    features    JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at  TIMESTAMPTZ DEFAULT now()
);

-- Build job state for white-label per-tenant Play Store / iOS apps.
CREATE TABLE IF NOT EXISTS app_builds (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    status        VARCHAR(20) NOT NULL DEFAULT 'queued',
    platform      VARCHAR(10) NOT NULL,
    package_id    VARCHAR(200),
    version_code  INTEGER,
    version_name  VARCHAR(20),
    build_url     TEXT,
    play_url      TEXT,
    error_log     TEXT,
    created_at    TIMESTAMPTZ DEFAULT now(),
    completed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_app_builds_tenant ON app_builds(tenant_id);
CREATE INDEX IF NOT EXISTS idx_app_builds_status ON app_builds(status);

-- Our billing of tenants (separate from the tenant's own student-facing
-- subscriptions table).
CREATE TABLE IF NOT EXISTS platform_subscriptions (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    plan                     VARCHAR(20) NOT NULL,
    status                   VARCHAR(20) NOT NULL DEFAULT 'trialing',
    current_period_end       TIMESTAMPTZ,
    razorpay_subscription_id VARCHAR(100),
    amount                   INTEGER NOT NULL DEFAULT 0,
    trial_ends_at            TIMESTAMPTZ,
    created_at               TIMESTAMPTZ DEFAULT now(),
    updated_at               TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_platform_subs_tenant ON platform_subscriptions(tenant_id);

-- Leads from the marketing website. Not tenant-scoped (tenants don't exist
-- yet at lead time).
CREATE TABLE IF NOT EXISTS leads (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(200),
    phone           VARCHAR(20),
    email           VARCHAR(200),
    institute_name  VARCHAR(200),
    city            VARCHAR(100),
    students_count  INTEGER,
    source          VARCHAR(50),
    status          VARCHAR(20) DEFAULT 'new',
    assigned_to     UUID,
    notes           TEXT,
    created_at      TIMESTAMPTZ DEFAULT now()
);

-- Seed a "default" tenant for backfill. Existing data gets attached to this
-- tenant in migration 028 so RLS doesn't orphan rows. Org code DEFAULT is
-- intentionally human-readable so the dev team can log in without checking
-- a UUID.
INSERT INTO tenants (id, org_code, name, slug, plan, status)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'DEFAULT',
    'Default Tenant',
    'default',
    'starter',
    'active'
)
ON CONFLICT (id) DO NOTHING;

-- Seed feature flags row for the default tenant so feature checks short-circuit
-- to "all on" during the migration window.
INSERT INTO tenant_features (tenant_id, features)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    '{"live": true, "store": true, "tests": true, "ai_doubts": true, "downloads": true}'::jsonb
)
ON CONFLICT (tenant_id) DO NOTHING;
