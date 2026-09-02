-- 0110_rls_policies.sql
-- Cross-table policies that need every domain table to already exist, plus
-- a hard assertion gate: if any table with a NOT NULL tenant_id is missing
-- ENABLE+FORCE RLS or either standard policy, this migration FAILS.

-- ─────────────────────────────────── users: "shares my current tenant"
DROP POLICY IF EXISTS users_self ON users;
DROP POLICY IF EXISTS users_visible ON users;
CREATE POLICY users_visible ON users
    USING (
        id = current_app_user()
        OR is_super_admin()
        OR EXISTS (
            SELECT 1 FROM tenant_users tu
            WHERE tu.user_id = users.id
              AND tu.tenant_id = current_tenant_id()
              AND tu.deleted_at IS NULL
        )
    )
    WITH CHECK (id = current_app_user() OR is_super_admin());

-- ─────────────────────────────────── assertion: standard tenant RLS
DO $$
DECLARE
    t       text;
    rls     boolean;
    problems text[] := '{}';
BEGIN
    FOR t IN
        SELECT DISTINCT c.table_name
        FROM information_schema.columns c
        JOIN pg_class pc ON pc.relname = c.table_name
        JOIN pg_namespace n ON n.oid = pc.relnamespace AND n.nspname = 'public'
        WHERE c.table_schema = 'public'
          AND c.column_name = 'tenant_id'
          AND c.is_nullable = 'NO'
          AND pc.relkind IN ('r','p')
          -- Identity tables carry tenant_id for scoping the session but are
          -- owner-only, not tenant-wide readable.
          AND c.table_name NOT IN ('refresh_tokens')
    LOOP
        SELECT relrowsecurity AND relforcerowsecurity INTO rls
        FROM pg_class WHERE relname = t AND relnamespace = 'public'::regnamespace;

        IF NOT COALESCE(rls, false) THEN
            problems := problems || (t || ': RLS not enabled+forced');
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_policies
                       WHERE schemaname='public' AND tablename=t
                         AND policyname = 'tenant_isolation_' || t) THEN
            problems := problems || (t || ': missing tenant_isolation_ policy');
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_policies
                       WHERE schemaname='public' AND tablename=t
                         AND policyname = 'super_admin_' || t) THEN
            problems := problems || (t || ': missing super_admin_ policy');
        END IF;
    END LOOP;

    IF array_length(problems, 1) > 0 THEN
        RAISE EXCEPTION E'RLS assertion failed:\n  - %',
            array_to_string(problems, E'\n  - ');
    END IF;
END $$;

-- ─────────────────────────────────── assertion: app_user DML everywhere
DO $$
DECLARE
    t        text;
    problems text[] := '{}';
BEGIN
    FOR t IN
        SELECT c.relname FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
        WHERE c.relkind IN ('r','p')
          AND c.relname <> 'schema_migrations_applied'
    LOOP
        IF NOT (has_table_privilege('app_user', format('public.%I', t), 'SELECT')
            AND has_table_privilege('app_user', format('public.%I', t), 'INSERT')
            AND has_table_privilege('app_user', format('public.%I', t), 'UPDATE')
            AND has_table_privilege('app_user', format('public.%I', t), 'DELETE')) THEN
            problems := problems || t;
        END IF;
    END LOOP;

    IF array_length(problems, 1) > 0 THEN
        RAISE EXCEPTION 'app_user missing DML on: %', array_to_string(problems, ', ');
    END IF;
END $$;

-- ─────────────────────────────────── updated_at backstop re-scan
DO $$
DECLARE t text;
BEGIN
    FOR t IN
        SELECT table_name FROM information_schema.columns
        WHERE table_schema='public' AND column_name='updated_at'
    LOOP
        PERFORM apply_updated_at(format('public.%I', t)::regclass);
    END LOOP;
END $$;
