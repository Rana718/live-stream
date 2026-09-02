-- 0030_taxonomy.sql
-- Reusable syllabus taxonomy, decoupled from course structure (a course's
-- ordered curriculum is course_sections/course_lessons in 0040). subjects
-- no longer belong to a single course — they are tenant-level and optionally
-- tied to an exam category.

-- ──────────────────────────────────────────────────────── exam_categories
-- Platform-level reference data (JEE, NEET, UPSC, …). Publicly readable;
-- only platform staff write. Supports one level of nesting.
CREATE TABLE exam_categories (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id     uuid REFERENCES exam_categories(id) ON DELETE SET NULL,
    name          text NOT NULL,
    slug          citext NOT NULL UNIQUE,
    description   text,
    icon_url      text,
    display_order integer NOT NULL DEFAULT 0,
    is_active     boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_exam_categories_parent ON exam_categories (parent_id);

ALTER TABLE exam_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE exam_categories FORCE ROW LEVEL SECURITY;
CREATE POLICY exam_categories_read ON exam_categories FOR SELECT USING (true);
CREATE POLICY exam_categories_super ON exam_categories
    USING (is_super_admin()) WITH CHECK (is_super_admin());
SELECT apply_updated_at('exam_categories');
SELECT apply_app_grants('exam_categories');

-- ─────────────────────────────────────────────────────────────── subjects
CREATE TABLE subjects (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    exam_category_id uuid REFERENCES exam_categories(id) ON DELETE SET NULL,
    name             text NOT NULL,
    code             text,
    icon_url         text,
    display_order    integer NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz
);
CREATE INDEX idx_subjects_tenant ON subjects (tenant_id);
CREATE INDEX idx_subjects_exam_category ON subjects (exam_category_id);
SELECT apply_tenant_table('subjects');

-- ─────────────────────────────────────────────────────────────── chapters
CREATE TABLE chapters (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    subject_id    uuid NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    name          text NOT NULL,
    description   text,
    display_order integer NOT NULL DEFAULT 0,
    is_free       boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);
CREATE INDEX idx_chapters_tenant ON chapters (tenant_id);
CREATE INDEX idx_chapters_subject ON chapters (subject_id, display_order);
SELECT apply_tenant_table('chapters');

-- ───────────────────────────────────────────────────────────────── topics
CREATE TABLE topics (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    chapter_id    uuid NOT NULL REFERENCES chapters(id) ON DELETE CASCADE,
    name          text NOT NULL,
    description   text,
    display_order integer NOT NULL DEFAULT 0,
    is_free       boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);
CREATE INDEX idx_topics_tenant ON topics (tenant_id);
CREATE INDEX idx_topics_chapter ON topics (chapter_id, display_order);
SELECT apply_tenant_table('topics');
