-- 046_integrity_and_money_fixes.sql
--
-- Corrective migration for production-readiness gaps found in the schema
-- audit. All statements are idempotent and safe to re-run.
--
-- Contents:
--   1. Drop the leftover GLOBAL unique constraint on users.email — it
--      silently blocks two orgs from having a user with the same email.
--      Per-tenant uniqueness is already enforced by
--      idx_users_tenant_email_unique (migration 028).
--   2. Protect financial history — a payments row must outlive the user it
--      belongs to. The app soft-deletes users (see users.DeleteUser); this
--      FK is the backstop against an accidental hard DELETE wiping payment
--      records.
--   3. Money sanity — non-negative amounts + a bounded status domain on
--      payments so a bad write fails loudly instead of corrupting revenue
--      reports.
--   4. Indexes for the revenue/refunds dashboards and for order-id lookups.
--   5. Auto-maintain updated_at on every table that has the column (only
--      the search-vector triggers existed before; updated_at was
--      app-managed and frequently stale).

BEGIN;

-- 1. ---------------------------------------------------------------------
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;

-- 2. ---------------------------------------------------------------------
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_user_id_fkey;
ALTER TABLE payments
    ADD CONSTRAINT payments_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;

-- 3. ---------------------------------------------------------------------
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_amount_nonneg;
ALTER TABLE payments
    ADD CONSTRAINT payments_amount_nonneg CHECK (amount >= 0);

-- NOTE: the codebase currently uses several synonyms for "money received"
-- (paid / captured / completed) across the course, subscription and fees
-- flows. This CHECK accepts the whole in-use vocabulary rather than force
-- a risky data migration; PRODUCTION-READINESS.md tracks collapsing these
-- to a single canonical set.
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_status_valid;
ALTER TABLE payments
    ADD CONSTRAINT payments_status_valid
    CHECK (status IN (
        'created','pending','authorized','paid','captured','completed',
        'failed','cancelled','refunded','partially_refunded'
    ));

-- 4. ---------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_payments_tenant_status_created
    ON payments (tenant_id, status, created_at DESC);

-- One live payment row per Razorpay order. Created NOT VALID-free because
-- payments has no historical duplicates; if a future prod run has dupes,
-- split this into a cleanup + CREATE UNIQUE INDEX CONCURRENTLY.
CREATE UNIQUE INDEX IF NOT EXISTS uq_payments_provider_order_id
    ON payments (provider_order_id)
    WHERE provider_order_id IS NOT NULL AND provider_order_id <> '';

-- 5. ---------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END $$;

DO $$
DECLARE
    t text;
BEGIN
    FOR t IN
        SELECT table_name
        FROM information_schema.columns
        WHERE table_schema = 'public' AND column_name = 'updated_at'
    LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS trg_%I_updated_at ON %I', t, t);
        EXECUTE format(
            'CREATE TRIGGER trg_%I_updated_at BEFORE UPDATE ON %I '
            'FOR EACH ROW EXECUTE FUNCTION set_updated_at()', t, t);
    END LOOP;
END $$;

COMMIT;
