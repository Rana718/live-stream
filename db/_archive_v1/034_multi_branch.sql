-- 034_multi_branch.sql
-- Multi-branch support for the Enterprise tier. A coaching chain with
-- multiple physical centres ("branches") can model each centre as its own
-- tenant row, all hanging off a single parent tenant. The parent owns the
-- billing relationship + brand-wide content; branches inherit theme but
-- maintain their own student rosters, fee structures, and audit logs.
--
-- We deliberately keep the data model flat — no recursive CTE traversal at
-- query time. parent_tenant_id is just a pointer; cross-branch reporting
-- happens via JOIN, scoped to the parent in the platform-admin SQL only.

ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS parent_tenant_id UUID
        REFERENCES tenants(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_tenants_parent ON tenants(parent_tenant_id);

-- A single CHECK guards against a tenant becoming its own parent.
ALTER TABLE tenants
    DROP CONSTRAINT IF EXISTS tenants_no_self_parent;
ALTER TABLE tenants
    ADD CONSTRAINT tenants_no_self_parent
    CHECK (parent_tenant_id IS NULL OR parent_tenant_id <> id);
