-- 028_add_tenant_id.sql
-- Add tenant_id to every business table. Three-step pattern per table:
--   1. ADD COLUMN tenant_id UUID (nullable initially)
--   2. UPDATE ... SET tenant_id = '<default>' WHERE tenant_id IS NULL
--   3. ALTER COLUMN tenant_id SET NOT NULL + add FK + composite index
--
-- Tables intentionally not tenant-scoped:
--   - tenants, tenant_users, tenant_features, app_builds, platform_subscriptions
--     (these are the multi-tenant control plane)
--   - leads (lead capture happens before a tenant exists)
--   - sms_codes (keyed on phone, not on a tenant — used during pre-auth OTP)
--   - exam_categories (curriculum metadata that's shared platform-wide)

-- Helper macro: do everything for a single table.
-- (Postgres doesn't have macros; we just spell it out per table for readability.)

DO $$
DECLARE
    default_tenant CONSTANT UUID := '00000000-0000-0000-0000-000000000001';
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
        'lecture_views'
    ];
BEGIN
    FOREACH t IN ARRAY tenant_scoped LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS tenant_id UUID', t);
        EXECUTE format('UPDATE %I SET tenant_id = $1 WHERE tenant_id IS NULL', t)
            USING default_tenant;
        EXECUTE format('ALTER TABLE %I ALTER COLUMN tenant_id SET NOT NULL', t);
        -- FK is added separately so we can ON DELETE CASCADE consistently.
        BEGIN
            EXECUTE format(
                'ALTER TABLE %I ADD CONSTRAINT fk_%s_tenant '
                'FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE',
                t, t
            );
        EXCEPTION WHEN duplicate_object THEN
            -- already exists, skip
            NULL;
        END;
    END LOOP;
END$$;

-- Composite indexes — every "by user" or "by created_at" query benefits from
-- being prefixed with tenant_id so RLS plus the index align.
CREATE INDEX IF NOT EXISTS idx_users_tenant_email
    ON users(tenant_id, email);
CREATE INDEX IF NOT EXISTS idx_courses_tenant_created
    ON courses(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_lectures_tenant_created
    ON lectures(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_streams_tenant_status
    ON streams(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_enrollments_tenant_user
    ON enrollments(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_tenant_user
    ON notifications(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_test_attempts_tenant_user
    ON test_attempts(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_chat_messages_tenant_stream
    ON chat_messages(tenant_id, stream_id);
CREATE INDEX IF NOT EXISTS idx_attendance_tenant_user
    ON attendance(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_payments_tenant_user
    ON payments(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_created
    ON audit_logs(tenant_id, created_at DESC);

-- Drop email-uniqueness if it was global (we recreate it per-tenant).
-- Email can repeat across tenants since each tenant is its own org.
DROP INDEX IF EXISTS idx_users_email;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_tenant_email_unique
    ON users(tenant_id, lower(email));

-- Backfill the user→tenant mapping in tenant_users for existing rows.
INSERT INTO tenant_users (tenant_id, user_id, role, status, joined_at)
SELECT
    '00000000-0000-0000-0000-000000000001',
    u.id,
    COALESCE(u.role, 'student'),
    'active',
    COALESCE(u.created_at, now())
FROM users u
ON CONFLICT (tenant_id, user_id, role) DO NOTHING;

-- Set the default tenant's owner_user_id to the first admin we find, so
-- /tenants/me works post-migration without manual intervention.
UPDATE tenants
SET owner_user_id = (
    SELECT id FROM users WHERE role = 'admin' ORDER BY created_at ASC LIMIT 1
)
WHERE id = '00000000-0000-0000-0000-000000000001'
  AND owner_user_id IS NULL;
