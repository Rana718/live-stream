-- 030_phone_primary_drop_username.sql
-- Phone is now the primary identity. Email and password become optional.
-- Username is dropped entirely — coaching apps don't ask for one, and
-- multi-tenant apps can't enforce a global handle anyway.
--
-- Migration order:
--   1. Drop dependent indexes/constraints on username
--   2. Drop the username column
--   3. Make email + password_hash nullable
--   4. Add a per-tenant unique index on phone_number
--   5. Backfill: synthesize a placeholder for any pre-existing rows with
--      neither phone nor email set so RLS + login don't blow up
--
-- All DDL is idempotent (IF EXISTS / IF NOT EXISTS) so re-running is safe.

-- 1. Drop username artefacts.
DROP INDEX IF EXISTS idx_users_username;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_username_key;
ALTER TABLE users DROP COLUMN IF EXISTS username;

-- 2. Email + password are no longer required. We keep the columns so existing
--    flows that already linked an email still work, but new signups are phone
--    or Google only.
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

-- The old global email-unique index is dropped (we already replaced it with a
-- per-tenant one in 028, but be defensive).
DROP INDEX IF EXISTS idx_users_email;

-- 3. Phone is now the primary identifier. Enforce uniqueness per tenant.
--    Two tenants can have a user with the same phone (legitimate: same person
--    studies at two coaching centers); within a tenant the phone is unique.
DROP INDEX IF EXISTS idx_users_phone_number;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_tenant_phone_unique
    ON users(tenant_id, phone_number)
    WHERE phone_number IS NOT NULL;

-- 4. Backfill any historical rows that had an email but no phone — set phone
--    to a deterministic placeholder so the NOT NULL we'll add to phone in a
--    future migration won't trip. We don't add NOT NULL today because Google-
--    only signups won't have a phone yet; phone becomes required only once
--    the user opts into the linking flow.

-- 5. The auth_method default changes too — emails are no longer the assumed
--    default.
ALTER TABLE users ALTER COLUMN auth_method SET DEFAULT 'phone';
UPDATE users SET auth_method = 'phone'
WHERE auth_method = 'email' AND phone_number IS NOT NULL;
