-- 039_course_bundles.sql
-- Replaces the subscription model for content access. Tenants now sell
-- access course-by-course, with optional admin-curated "combos" — buy 2
-- or 3 courses at a discount.
--
-- We keep `subscription_plans` and `user_subscriptions` around: they're
-- still useful for non-content recurring billing (e.g. doubt-solving
-- credits) and ripping them out churns a lot of code that already
-- compiles. The new student UX simply doesn't surface them.
--
-- Schema:
--   course_bundles       — one row per combo offering
--   course_bundle_items  — link rows (bundle_id, course_id), N-to-N
--
-- Pricing is stored in paise (integer) on the bundle row, NOT computed
-- from member courses. Admins set the bundle price directly so they
-- can advertise a clean number ("3 courses for ₹2,499") without us
-- having to calculate-and-round percentage discounts at request time.

CREATE TABLE IF NOT EXISTS course_bundles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title        VARCHAR(200) NOT NULL,
    description  TEXT,
    -- Bundle price in paise. Single source of truth — the per-course
    -- "price you would have paid" is computed in the API layer for the
    -- "save ₹X" sticker.
    price_paise  INTEGER NOT NULL CHECK (price_paise >= 0),
    cover_url    TEXT,
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    -- Sort key for ordering on the student store page. Lower first.
    -- Defaults to created_at-derived so new bundles appear at the bottom
    -- without admin work; adjust to feature a bundle.
    display_order INTEGER NOT NULL DEFAULT 100,
    created_at   TIMESTAMPTZ DEFAULT now(),
    updated_at   TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_course_bundles_tenant
    ON course_bundles(tenant_id, is_active, display_order);

CREATE TABLE IF NOT EXISTS course_bundle_items (
    bundle_id   UUID NOT NULL REFERENCES course_bundles(id) ON DELETE CASCADE,
    course_id   UUID NOT NULL REFERENCES courses(id)        ON DELETE CASCADE,
    PRIMARY KEY (bundle_id, course_id)
);

CREATE INDEX IF NOT EXISTS idx_bundle_items_course ON course_bundle_items(course_id);

-- RLS — same shape as every other tenant-scoped table. Tenant isolation
-- via the session var, super_admin bypass via the platform_is_super
-- function set by SuperAdminContext.
ALTER TABLE course_bundles ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS course_bundles_tenant_isolation ON course_bundles;
CREATE POLICY course_bundles_tenant_isolation ON course_bundles
    USING (
        tenant_id = current_tenant_id()
        OR is_super_admin()
    )
    WITH CHECK (
        tenant_id = current_tenant_id()
        OR is_super_admin()
    );

-- The link table doesn't carry tenant_id (it's derivable via the bundle
-- it joins on). We still gate by the parent's RLS via a join-side check
-- — anyone who can SELECT the bundle can see its items.
ALTER TABLE course_bundle_items ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS course_bundle_items_via_bundle ON course_bundle_items;
CREATE POLICY course_bundle_items_via_bundle ON course_bundle_items
    USING (
        EXISTS (
            SELECT 1 FROM course_bundles b
            WHERE b.id = course_bundle_items.bundle_id
              AND (b.tenant_id = current_tenant_id() OR is_super_admin())
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1 FROM course_bundles b
            WHERE b.id = course_bundle_items.bundle_id
              AND (b.tenant_id = current_tenant_id() OR is_super_admin())
        )
    );

-- The course_orders schema (migration 031) already has a generic `metadata`
-- jsonb. We piggy-back on it for bundle purchases — the order row stores
-- `metadata->>'bundle_id'` and the verify step expands the membership to
-- enroll the student in every course of the bundle. No schema change to
-- course_orders is required.
