-- 043_restricted_app_role.sql
--
-- CRITICAL: every RLS policy from 029_rls_policies.sql has been inert in
-- every environment since it was written. The application has only ever
-- connected to Postgres as the `postgres` role — a superuser — and
-- Postgres superusers unconditionally bypass row-level security. This is
-- hardcoded Postgres behavior; no policy, GRANT, or FORCE ROW LEVEL
-- SECURITY can override it. Confirmed via
-- `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'postgres'`
-- returning (true, true), and empirically via
-- internal/middleware/tenant_isolation_test.go, which failed with a
-- cross-tenant read the first time it was ever actually run to completion
-- against a fully-migrated database.
--
-- This creates a separate, unprivileged role for the application to
-- connect as. It owns nothing (postgres/the migration runner still owns
-- every table), has no BYPASSRLS, and is not a superuser — the ordinary
-- case row security is designed for. `postgres` remains the role that
-- runs migrations/backups/restores (scripts/migrate.sh,
-- scripts/backup-db.sh, scripts/restore-db.sh all default to it), since
-- DDL and admin operations should stay on an elevated, RLS-exempt account
-- separate from request-serving traffic.
--
-- The password below is a placeholder matching this repo's existing
-- convention for docker-compose dev credentials (POSTGRES_PASSWORD is
-- also literally "postgres" — neither is meant for production). Rotate it
-- the same way as every other production secret — see
-- docs/runbooks/rotate-secrets.md — before this ever runs against a real
-- deployment; don't reuse the placeholder outside local dev.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'app_user') THEN
        CREATE ROLE app_user WITH LOGIN PASSWORD 'app_user_dev_password' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
END
$$;

GRANT CONNECT ON DATABASE live_platform TO app_user;
GRANT USAGE ON SCHEMA public TO app_user;

-- Every table that exists right now.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app_user;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_user;

-- Every table a *future* migration creates, automatically — without this,
-- migration 044 onward would silently need its own GRANT statement or the
-- app gets a permission-denied error on a table RLS should be protecting
-- it on, not exposing it.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO app_user;
