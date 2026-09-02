-- 029_rls_policies.sql
-- Row-level security on every tenant-scoped table.
--
-- Reads/writes only succeed if the connection has set:
--   SET LOCAL app.tenant_id = '<uuid>'
--
-- The Fiber TenantContext middleware sets this per-transaction. Background
-- jobs that operate cross-tenant (e.g. admin reports) set
--   SET LOCAL app.is_super_admin = 'true'
-- to bypass.

-- A single helper function avoids repeating the WHERE clause logic.
-- IMMUTABLE-ish: same input → same output within the connection lifetime.
CREATE OR REPLACE FUNCTION current_tenant_id()
RETURNS UUID
LANGUAGE sql
STABLE
AS $$
    SELECT nullif(current_setting('app.tenant_id', true), '')::uuid
$$;

CREATE OR REPLACE FUNCTION is_super_admin()
RETURNS BOOLEAN
LANGUAGE sql
STABLE
AS $$
    SELECT COALESCE(current_setting('app.is_super_admin', true) = 'true', false)
$$;

DO $$
DECLARE
    t TEXT;
    tenant_scoped TEXT[] := ARRAY[
        'users',
        'streams',
        'recordings',
        'chat_messages',
        'courses',
        'batches',
        'subjects',
        'chapters',
        'topics',
        'lectures',
        'study_materials',
        'tests',
        'questions',
        'question_options',
        'test_attempts',
        'test_answers',
        'doubts',
        'doubt_answers',
        'subscription_plans',
        'user_subscriptions',
        'payments',
        'enrollments',
        'notifications',
        'announcements',
        'attendance',
        'class_qr_codes',
        'assignments',
        'assignment_submissions',
        'fee_structures',
        'student_fees',
        'fee_installments',
        'bookmarks',
        'video_variants',
        'download_tokens',
        'audit_logs',
        'banners',
        'lecture_views',
        'tenant_users'
    ];
BEGIN
    FOREACH t IN ARRAY tenant_scoped LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);

        -- Drop old policies if re-running.
        EXECUTE format('DROP POLICY IF EXISTS %I ON %I', 'tenant_isolation_' || t, t);
        EXECUTE format('DROP POLICY IF EXISTS %I ON %I', 'super_admin_' || t, t);

        -- Tenant isolation: any row whose tenant_id matches the session var.
        EXECUTE format(
            'CREATE POLICY %I ON %I '
            'USING (tenant_id = current_tenant_id()) '
            'WITH CHECK (tenant_id = current_tenant_id())',
            'tenant_isolation_' || t, t
        );

        -- Bypass for super-admin operations (set per-transaction).
        EXECUTE format(
            'CREATE POLICY %I ON %I '
            'USING (is_super_admin()) '
            'WITH CHECK (is_super_admin())',
            'super_admin_' || t, t
        );
    END LOOP;
END$$;

-- Tenants table itself: super-admin only for writes; reads allowed by anyone
-- with the matching tenant_id (so a regular user can fetch their own tenant
-- record, useful for theming).
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_self_read ON tenants;
DROP POLICY IF EXISTS tenant_super_admin ON tenants;
DROP POLICY IF EXISTS tenant_public_orgcode ON tenants;

-- Bypass for super-admin (e.g. cron + provisioning workers).
CREATE POLICY tenant_super_admin ON tenants
    USING (is_super_admin())
    WITH CHECK (is_super_admin());

-- Authenticated user can SELECT their own tenant row.
CREATE POLICY tenant_self_read ON tenants
    FOR SELECT
    USING (id = current_tenant_id() OR is_super_admin());

-- Public org-code lookup (used by /public/tenants/by-code/:code):
-- We allow SELECT even without app.tenant_id set, but only of the row whose
-- org_code is being looked up via a where-clause. The handler sets
--   app.is_public_lookup = 'true'
-- to opt into this policy.
CREATE POLICY tenant_public_orgcode ON tenants
    FOR SELECT
    USING (COALESCE(current_setting('app.is_public_lookup', true) = 'true', false));

-- tenant_users gets simple isolation already via the loop above. Add a
-- secondary policy so a user can read their own membership rows even before
-- the app.tenant_id is known (login flow needs this to pick a tenant).
DROP POLICY IF EXISTS tenant_users_self_read ON tenant_users;
CREATE POLICY tenant_users_self_read ON tenant_users
    FOR SELECT
    USING (
        user_id = nullif(current_setting('app.user_id', true), '')::uuid
        OR is_super_admin()
    );
