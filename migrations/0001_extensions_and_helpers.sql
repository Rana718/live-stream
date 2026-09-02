-- 0001_extensions_and_helpers.sql
-- Schema v2 foundation. No business tables — extensions, the restricted
-- application role, RLS session helpers, and the per-table helper functions
-- every later domain migration calls at its tail.
--
-- Runs as the migration role (postgres / superuser). The application
-- connects as `app_user` (NOSUPERUSER NOBYPASSRLS) — RLS is the entire
-- tenant-isolation mechanism and a superuser connection silently disables
-- it (see db/_archive_v1/043_restricted_app_role.sql for the full story;
-- internal/database/postgres.go refuses to start against a BYPASSRLS role).

-- ─────────────────────────────────────────────────────────── extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;     -- case-insensitive email / codes
CREATE EXTENSION IF NOT EXISTS pg_trgm;    -- trigram search indexes

-- ─────────────────────────────────────────────────────── application role
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'app_user') THEN
        CREATE ROLE app_user WITH LOGIN PASSWORD 'app_user_dev_password'
            NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
END $$;

DO $$
BEGIN
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO app_user', current_database());
END $$;
GRANT USAGE ON SCHEMA public TO app_user;

-- Future tables/sequences created by the migration role are granted to
-- app_user automatically. Partitions created later by a worker are handled
-- by create_month_partition() below (SECURITY DEFINER).
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO app_user;

-- ──────────────────────────────────────────────────── RLS session helpers
-- The pgxpool BeforeAcquire hook (internal/database/postgres.go) runs
-- set_config('app.tenant_id' | 'app.user_id' | 'app.is_super_admin' |
-- 'app.is_public_lookup', …, false) on every query from the request ctx.

CREATE OR REPLACE FUNCTION current_tenant_id() RETURNS uuid
    LANGUAGE sql STABLE PARALLEL SAFE AS
$$ SELECT nullif(current_setting('app.tenant_id', true), '')::uuid $$;

CREATE OR REPLACE FUNCTION current_app_user() RETURNS uuid
    LANGUAGE sql STABLE PARALLEL SAFE AS
$$ SELECT nullif(current_setting('app.user_id', true), '')::uuid $$;

CREATE OR REPLACE FUNCTION is_super_admin() RETURNS boolean
    LANGUAGE sql STABLE PARALLEL SAFE AS
$$ SELECT COALESCE(current_setting('app.is_super_admin', true) = 'true', false) $$;

CREATE OR REPLACE FUNCTION is_public_lookup() RETURNS boolean
    LANGUAGE sql STABLE PARALLEL SAFE AS
$$ SELECT COALESCE(current_setting('app.is_public_lookup', true) = 'true', false) $$;

-- ──────────────────────────────────────────────────────── updated_at
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger
    LANGUAGE plpgsql AS
$$ BEGIN NEW.updated_at = now(); RETURN NEW; END $$;

-- apply_updated_at(t): attach the BEFORE UPDATE trigger. No-op if the table
-- has no updated_at column.
CREATE OR REPLACE FUNCTION apply_updated_at(tbl regclass) RETURNS void
    LANGUAGE plpgsql SET client_min_messages = warning AS
$$
DECLARE
    short text := regexp_replace(tbl::text, '^.*\.', '');
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = trim(both '"' from short)
          AND column_name = 'updated_at'
    ) THEN
        RETURN;
    END IF;
    EXECUTE format('DROP TRIGGER IF EXISTS trg_%s_updated_at ON %s', short, tbl);
    EXECUTE format(
        'CREATE TRIGGER trg_%s_updated_at BEFORE UPDATE ON %s '
        'FOR EACH ROW EXECUTE FUNCTION set_updated_at()', short, tbl);
END $$;

-- ──────────────────────────────────────────────────────── app_user grants
-- Belt-and-braces over ALTER DEFAULT PRIVILEGES — explicit per table so a
-- table created by a non-migration role (a worker) is still reachable.
CREATE OR REPLACE FUNCTION apply_app_grants(tbl regclass) RETURNS void
    LANGUAGE plpgsql AS
$$
BEGIN
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %s TO app_user', tbl);
END $$;

-- ──────────────────────────────────────────────────────── tenant RLS
-- apply_tenant_rls(t): ENABLE + FORCE row security and (re)create the two
-- canonical policies. Permissive policies OR together, so a row is visible
-- when tenant_id matches the session tenant OR the session is super-admin.
-- Recurses into partitions so a direct partition query is also gated.
--
-- Role-scoped (student-only) visibility is enforced in the application /
-- sqlc query layer, not here — matching db/_archive_v1/029_rls_policies.sql.
CREATE OR REPLACE FUNCTION apply_tenant_rls(tbl regclass) RETURNS void
    LANGUAGE plpgsql SET client_min_messages = warning AS
$$
DECLARE
    short text := trim(both '"' from regexp_replace(tbl::text, '^.*\.', ''));
    child regclass;
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', tbl);
    EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', tbl);

    EXECUTE format('DROP POLICY IF EXISTS %I ON %s', 'tenant_isolation_' || short, tbl);
    EXECUTE format(
        'CREATE POLICY %I ON %s USING (tenant_id = current_tenant_id()) '
        'WITH CHECK (tenant_id = current_tenant_id())',
        'tenant_isolation_' || short, tbl);

    EXECUTE format('DROP POLICY IF EXISTS %I ON %s', 'super_admin_' || short, tbl);
    EXECUTE format(
        'CREATE POLICY %I ON %s USING (is_super_admin()) WITH CHECK (is_super_admin())',
        'super_admin_' || short, tbl);

    FOR child IN
        SELECT inhrelid::regclass FROM pg_inherits WHERE inhparent = tbl
    LOOP
        PERFORM apply_tenant_rls(child);
    END LOOP;
END $$;

-- One-shot for the common tail of a domain migration.
CREATE OR REPLACE FUNCTION apply_tenant_table(tbl regclass) RETURNS void
    LANGUAGE plpgsql SET client_min_messages = warning AS
$$
BEGIN
    PERFORM apply_tenant_rls(tbl);
    PERFORM apply_updated_at(tbl);
    PERFORM apply_app_grants(tbl);
END $$;

-- ──────────────────────────────────────────────────────── partitioning
-- create_month_partition(parent, month): create parent_YYYYMM covering
-- [month, month+1). SECURITY DEFINER so the scheduleworker (app_user) can
-- call it; grants app_user on the new partition explicitly.
CREATE OR REPLACE FUNCTION create_month_partition(parent regclass, month date)
    RETURNS void LANGUAGE plpgsql SECURITY DEFINER
    SET search_path = public
    SET client_min_messages = warning AS
$$
DECLARE
    p_short   text := trim(both '"' from regexp_replace(parent::text, '^.*\.', ''));
    from_ts   date := date_trunc('month', month)::date;
    to_ts     date := (date_trunc('month', month) + interval '1 month')::date;
    part_name text := format('%s_%s', p_short, to_char(from_ts, 'YYYYMM'));
    part      regclass;
    pol       record;
BEGIN
    IF to_regclass('public.' || part_name) IS NOT NULL THEN
        RETURN;
    END IF;
    EXECUTE format(
        'CREATE TABLE public.%I PARTITION OF %s FOR VALUES FROM (%L) TO (%L)',
        part_name, parent, from_ts, to_ts);
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO app_user', part_name);
    part := ('public.' || part_name)::regclass;

    -- Mirror the parent's row security onto the new partition so a direct
    -- partition query is gated identically (parent policies don't cascade).
    IF (SELECT relrowsecurity FROM pg_class WHERE oid = parent) THEN
        EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', part);
        EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', part);
        FOR pol IN
            SELECT p.polname,
                   CASE p.polcmd WHEN '*' THEN 'ALL' WHEN 'r' THEN 'SELECT'
                                 WHEN 'a' THEN 'INSERT' WHEN 'w' THEN 'UPDATE'
                                 WHEN 'd' THEN 'DELETE' END AS cmd,
                   pg_get_expr(p.polqual, p.polrelid)      AS using_expr,
                   pg_get_expr(p.polwithcheck, p.polrelid) AS check_expr
            FROM pg_policy p WHERE p.polrelid = parent
        LOOP
            EXECUTE format('DROP POLICY IF EXISTS %I ON %s',
                regexp_replace(pol.polname, '_' || p_short || '$', '_' || part_name), part);
            EXECUTE format('CREATE POLICY %I ON %s FOR %s%s%s',
                regexp_replace(pol.polname, '_' || p_short || '$', '_' || part_name),
                part, pol.cmd,
                CASE WHEN pol.using_expr IS NOT NULL THEN ' USING (' || pol.using_expr || ')' ELSE '' END,
                CASE WHEN pol.check_expr IS NOT NULL THEN ' WITH CHECK (' || pol.check_expr || ')' ELSE '' END);
        END LOOP;
    END IF;
END $$;

REVOKE ALL ON FUNCTION create_month_partition(regclass, date) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION create_month_partition(regclass, date) TO app_user;
