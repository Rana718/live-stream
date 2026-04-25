-- 028_add_tenant_id.sql
-- Add tenant_id to every business table. Three-step pattern per table:
--   1. ADD COLUMN tenant_id UUID (nullable initially)
--   2. UPDATE ... SET tenant_id = <default-tenant> WHERE NULL
--   3. ALTER COLUMN tenant_id SET NOT NULL + FK + composite index
--
-- We previously used a DO block + EXECUTE format() loop for the boilerplate
-- but sqlc couldn't see the column additions inside dynamic SQL — its
-- generated structs missed `TenantID` on every table. Spelling each ALTER
-- out is verbose but makes both goose and sqlc happy from the same file.
--
-- Tables intentionally NOT tenant-scoped:
--   tenants, tenant_users, tenant_features, app_builds,
--   platform_subscriptions (multi-tenant control plane)
--   leads (captured before any tenant exists)
--   sms_codes (keyed on phone, used pre-auth)
--   exam_categories (curriculum metadata shared platform-wide)

-- ─── 1. ADD COLUMN ─────────────────────────────────────────────────────────
ALTER TABLE users                  ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE streams                ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE recordings             ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE chat_messages          ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE courses                ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE batches                ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE subjects               ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE chapters               ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE topics                 ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE lectures               ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE study_materials        ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE tests                  ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE questions              ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE question_options       ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE test_attempts          ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE test_answers           ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE doubts                 ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE doubt_answers          ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE subscription_plans     ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE user_subscriptions     ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE payments               ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE enrollments            ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE notifications          ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE announcements          ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE attendance             ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE class_qr_codes         ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE assignments            ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE assignment_submissions ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE fee_structures         ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE student_fees           ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE fee_installments       ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE bookmarks              ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE video_variants         ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE download_tokens        ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE audit_logs             ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE banners                ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE lecture_views          ADD COLUMN IF NOT EXISTS tenant_id UUID;

-- ─── 2. BACKFILL ───────────────────────────────────────────────────────────
-- Existing rows attach to the seed default tenant created in 027.
UPDATE users                  SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE streams                SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE recordings             SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE chat_messages          SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE courses                SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE batches                SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE subjects               SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE chapters               SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE topics                 SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE lectures               SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE study_materials        SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE tests                  SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE questions              SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE question_options       SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE test_attempts          SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE test_answers           SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE doubts                 SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE doubt_answers          SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE subscription_plans     SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE user_subscriptions     SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE payments               SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE enrollments            SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE notifications          SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE announcements          SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE attendance             SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE class_qr_codes         SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE assignments            SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE assignment_submissions SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE fee_structures         SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE student_fees           SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE fee_installments       SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE bookmarks              SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE video_variants         SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE download_tokens        SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE audit_logs             SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE banners                SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE lecture_views          SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;

-- ─── 3. NOT NULL + FK ──────────────────────────────────────────────────────
ALTER TABLE users                  ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE streams                ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE recordings             ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE chat_messages          ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE courses                ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE batches                ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE subjects               ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE chapters               ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE topics                 ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE lectures               ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE study_materials        ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE tests                  ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE questions              ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE question_options       ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE test_attempts          ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE test_answers           ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE doubts                 ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE doubt_answers          ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE subscription_plans     ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE user_subscriptions     ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE payments               ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE enrollments            ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE notifications          ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE announcements          ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE attendance             ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE class_qr_codes         ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE assignments            ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE assignment_submissions ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE fee_structures         ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE student_fees           ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE fee_installments       ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE bookmarks              ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE video_variants         ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE download_tokens        ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE audit_logs             ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE banners                ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE lecture_views          ALTER COLUMN tenant_id SET NOT NULL;

-- FK adds happen in a DO block per-table so we can swallow the
-- "constraint already exists" error on re-runs without aborting the
-- migration. sqlc doesn't need to read these — the column already exists
-- by this point and FKs don't change column types.
DO $$
DECLARE
    t TEXT;
    tenant_scoped TEXT[] := ARRAY[
        'users', 'streams', 'recordings', 'chat_messages', 'courses', 'batches',
        'subjects', 'chapters', 'topics', 'lectures', 'study_materials',
        'tests', 'questions', 'question_options', 'test_attempts', 'test_answers',
        'doubts', 'doubt_answers', 'subscription_plans', 'user_subscriptions',
        'payments', 'enrollments', 'notifications', 'announcements', 'attendance',
        'class_qr_codes', 'assignments', 'assignment_submissions', 'fee_structures',
        'student_fees', 'fee_installments', 'bookmarks', 'video_variants',
        'download_tokens', 'audit_logs', 'banners', 'lecture_views'
    ];
BEGIN
    FOREACH t IN ARRAY tenant_scoped LOOP
        BEGIN
            EXECUTE format(
                'ALTER TABLE %I ADD CONSTRAINT fk_%s_tenant '
                'FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE',
                t, t
            );
        EXCEPTION WHEN duplicate_object THEN
            NULL;
        END;
    END LOOP;
END$$;

-- ─── 4. Composite indexes — query-pattern aligned with RLS ─────────────────
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
