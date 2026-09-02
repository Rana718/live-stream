-- 0132_audit_default_partition_rls.sql
-- audit_logs_default is created inline in 0100 *before* RLS is enabled on the
-- parent, and unlike the month partitions (built by create_month_partition,
-- which mirrors parent RLS) it never got its own policies. Direct access to
-- the default partition would therefore bypass tenant isolation. Mirror the
-- parent's bespoke nullable-tenant policies onto it.

ALTER TABLE audit_logs_default ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs_default FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS audit_logs_read ON audit_logs_default;
CREATE POLICY audit_logs_read ON audit_logs_default FOR SELECT
    USING (tenant_id = current_tenant_id() OR is_super_admin());

DROP POLICY IF EXISTS audit_logs_insert ON audit_logs_default;
CREATE POLICY audit_logs_insert ON audit_logs_default FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id() OR tenant_id IS NULL OR is_super_admin());
