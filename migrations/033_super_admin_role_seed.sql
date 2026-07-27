-- 033_super_admin_role_seed.sql
-- Establishes the `super_admin` role at the data layer.
--
-- Roles in this system:
--   super_admin — platform staff (you). Bypasses RLS via SuperAdminContext.
--   admin       — tenant admin (one per tenant org).
--   instructor  — tenant instructor.
--   student     — default for new sign-ups.
--   parent      — read-only view of a child student account (future).
--
-- The roles are stored as free-form text so we can introduce new roles
-- without DDL. To prevent typos shorting the auth chain we add a CHECK
-- constraint and a btree index for role-based filtering on /admin/users.
--
-- users.role was locked to a Postgres ENUM (user_role: student/admin/
-- instructor only) by migration 005, which conflicts with that "free-form
-- text" model — this migration predates super_admin existing as a value
-- and would fail on ALTER TABLE ... ADD CONSTRAINT below with "invalid
-- input value for enum user_role" the first time it's run against a truly
-- fresh database. Convert back to VARCHAR to match tenant_users.role
-- (already VARCHAR, see migration 027) before adding the CHECK. The Go
-- layer already treats this column as pgtype.Text, not a generated Go
-- enum — see the sqlc.yaml overrides — so this has no code-level impact.
ALTER TABLE users
    ALTER COLUMN role DROP DEFAULT,
    ALTER COLUMN role TYPE VARCHAR(20) USING role::text,
    ALTER COLUMN role SET DEFAULT 'student';
DROP TYPE IF EXISTS user_role;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users
    ADD CONSTRAINT users_role_check
    CHECK (role IN ('super_admin', 'admin', 'instructor', 'student', 'parent'));

CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);

-- The `tenant_users` membership table mirrors role per-tenant. super_admin
-- doesn't normally have a tenant_users row — they operate cross-tenant —
-- but we accept it in the constraint so dev/test fixtures can attach a
-- super_admin to the default tenant for quick local testing.
ALTER TABLE tenant_users
    DROP CONSTRAINT IF EXISTS tenant_users_role_check;
ALTER TABLE tenant_users
    ADD CONSTRAINT tenant_users_role_check
    CHECK (role IN ('super_admin', 'owner', 'admin', 'instructor', 'student', 'parent'));
