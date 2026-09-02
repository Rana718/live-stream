-- 0040_catalog_content.sql
-- Sellable programs (courses), their ordered curriculum (sections/lessons),
-- the typed content bodies a lesson points at, live sessions + recordings,
-- scheduling, progress and bookmarks. No prices here — pricing is 0060.

-- ─────────────────────────────────────────────────────────────── courses
CREATE TABLE courses (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    exam_category_id  uuid REFERENCES exam_categories(id) ON DELETE SET NULL,
    title             text NOT NULL,
    slug              citext NOT NULL,
    summary           text,
    description_rich  jsonb NOT NULL DEFAULT '{}'::jsonb,
    thumbnail_url     text,
    promo_video_url   text,
    language          text NOT NULL DEFAULT 'en',
    level             text NOT NULL DEFAULT 'foundation',
    class_level       text,
    exam_goal         text,
    status            publish_status NOT NULL DEFAULT 'draft',
    approval_status   text NOT NULL DEFAULT 'pending'
                          CHECK (approval_status IN ('pending','approved','rejected')),
    approved_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    approved_at       timestamptz,
    rejection_reason  text,
    hsn_sac           text,
    tax_rate_bps      integer NOT NULL DEFAULT 0 CHECK (tax_rate_bps BETWEEN 0 AND 10000),
    refund_window_days integer NOT NULL DEFAULT 0 CHECK (refund_window_days >= 0),
    starts_on         date,
    ends_on           date,
    seats             integer,
    created_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    search_vector     tsvector GENERATED ALWAYS AS (
                          to_tsvector('english'::regconfig,
                              coalesce(title,'') || ' ' || coalesce(summary,''))
                      ) STORED,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    deleted_at        timestamptz
);
CREATE UNIQUE INDEX uq_courses_tenant_slug ON courses (tenant_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX idx_courses_tenant_status ON courses (tenant_id, status);
CREATE INDEX idx_courses_exam_category ON courses (exam_category_id);
CREATE INDEX idx_courses_search ON courses USING gin (search_vector);
SELECT apply_tenant_table('courses');

-- ─────────────────────────────────────────────────────────────── batches
-- A cohort of a course: its own roster, schedule and fee plan.
CREATE TABLE batches (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id     uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    name          text NOT NULL,
    description   text,
    instructor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    starts_on     date,
    ends_on       date,
    max_students  integer,
    is_active     boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);
CREATE INDEX idx_batches_tenant ON batches (tenant_id);
CREATE INDEX idx_batches_course ON batches (course_id);
CREATE INDEX idx_batches_instructor ON batches (instructor_id);
SELECT apply_tenant_table('batches');

-- ─────────────────────────────────────────────────────── course_instructors
CREATE TABLE course_instructors (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id         uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role              text NOT NULL DEFAULT 'instructor'
                          CHECK (role IN ('owner','instructor','ta')),
    revenue_share_bps integer NOT NULL DEFAULT 0 CHECK (revenue_share_bps BETWEEN 0 AND 10000),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (course_id, user_id)
);
CREATE INDEX idx_course_instructors_tenant ON course_instructors (tenant_id);
CREATE INDEX idx_course_instructors_user ON course_instructors (user_id);
SELECT apply_tenant_table('course_instructors');

-- ──────────────────────────────────────────────────────── course_sections
CREATE TABLE course_sections (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id       uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    title           text NOT NULL,
    display_order   integer NOT NULL DEFAULT 0,
    drip_after_days integer NOT NULL DEFAULT 0 CHECK (drip_after_days >= 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);
CREATE INDEX idx_course_sections_course ON course_sections (course_id, display_order);
CREATE INDEX idx_course_sections_tenant ON course_sections (tenant_id);
SELECT apply_tenant_table('course_sections');

-- ─────────────────────────────────────────────────────── content bodies
CREATE TABLE content_videos (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    title           text NOT NULL,
    provider        text NOT NULL DEFAULT 'self' CHECK (provider IN ('self','mux','bunny','youtube')),
    playback_id     text,
    duration_sec    integer NOT NULL DEFAULT 0,
    drm             boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);
CREATE INDEX idx_content_videos_tenant ON content_videos (tenant_id);
SELECT apply_tenant_table('content_videos');

CREATE TABLE content_documents (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    title       text NOT NULL,
    file_key    text NOT NULL,
    file_size   bigint NOT NULL DEFAULT 0,
    mime        text,
    page_count  integer,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
CREATE INDEX idx_content_documents_tenant ON content_documents (tenant_id);
SELECT apply_tenant_table('content_documents');

CREATE TABLE content_links (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    title       text NOT NULL,
    url         text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
CREATE INDEX idx_content_links_tenant ON content_links (tenant_id);
SELECT apply_tenant_table('content_links');

-- ─────────────────────────────────────────────────────── video renditions
CREATE TABLE video_assets (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    source_key   text,
    status       recording_status NOT NULL DEFAULT 'queued',
    duration_sec integer NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_video_assets_tenant ON video_assets (tenant_id);
SELECT apply_tenant_table('video_assets');

CREATE TABLE video_renditions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    video_asset_id uuid NOT NULL REFERENCES video_assets(id) ON DELETE CASCADE,
    height         integer NOT NULL,
    bitrate_kbps   integer NOT NULL DEFAULT 0,
    codec          text NOT NULL DEFAULT 'h264',
    file_key       text NOT NULL,
    file_size      bigint NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (video_asset_id, height)
);
CREATE INDEX idx_video_renditions_tenant ON video_renditions (tenant_id);
SELECT apply_tenant_table('video_renditions');

-- ────────────────────────────────────────────────────────── class_schedules
CREATE TABLE class_schedules (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id           uuid REFERENCES courses(id) ON DELETE SET NULL,
    batch_id            uuid REFERENCES batches(id) ON DELETE SET NULL,
    instructor_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title               text NOT NULL,
    description         text,
    by_weekday          smallint[] NOT NULL DEFAULT '{}',
    start_local         text NOT NULL,           -- "HH:MM"
    duration_min        integer NOT NULL DEFAULT 60,
    timezone            text NOT NULL DEFAULT 'Asia/Kolkata',
    starts_on           date NOT NULL DEFAULT current_date,
    ends_on             date,
    is_active           boolean NOT NULL DEFAULT true,
    last_materialised_at timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_class_schedules_tenant_active ON class_schedules (tenant_id, is_active) WHERE is_active;
CREATE INDEX idx_class_schedules_instructor ON class_schedules (instructor_id);
CREATE INDEX idx_class_schedules_course ON class_schedules (course_id);
CREATE INDEX idx_class_schedules_batch ON class_schedules (batch_id);
SELECT apply_tenant_table('class_schedules');

-- ──────────────────────────────────────────────────────────── live_sessions
CREATE TABLE live_sessions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id       uuid REFERENCES courses(id) ON DELETE SET NULL,
    batch_id        uuid REFERENCES batches(id) ON DELETE SET NULL,
    instructor_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    schedule_id     uuid REFERENCES class_schedules(id) ON DELETE SET NULL,
    title           text NOT NULL,
    description     text,
    status          session_status NOT NULL DEFAULT 'scheduled',
    ingest_key      text NOT NULL UNIQUE,
    scheduled_start timestamptz,
    actual_start    timestamptz,
    actual_end      timestamptz,
    peak_viewers    integer NOT NULL DEFAULT 0,
    thumbnail_url   text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);
CREATE INDEX idx_live_sessions_tenant_status ON live_sessions (tenant_id, status);
CREATE INDEX idx_live_sessions_course ON live_sessions (course_id);
CREATE INDEX idx_live_sessions_batch ON live_sessions (batch_id);
CREATE INDEX idx_live_sessions_schedule ON live_sessions (schedule_id);
SELECT apply_tenant_table('live_sessions');

-- ──────────────────────────────────────────────────────────── recordings
CREATE TABLE recordings (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    session_id     uuid REFERENCES live_sessions(id) ON DELETE SET NULL,
    video_asset_id uuid REFERENCES video_assets(id) ON DELETE SET NULL,
    file_key       text,
    file_size      bigint,
    duration_sec   integer,
    status         recording_status NOT NULL DEFAULT 'queued',
    thumbnail_url  text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_recordings_tenant ON recordings (tenant_id);
CREATE INDEX idx_recordings_session ON recordings (session_id);
SELECT apply_tenant_table('recordings');

-- ─────────────────────────────────────────────────────────── course_lessons
CREATE TABLE course_lessons (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id           uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    section_id          uuid REFERENCES course_sections(id) ON DELETE SET NULL,
    title               text NOT NULL,
    content_kind        content_kind NOT NULL,
    video_id            uuid REFERENCES content_videos(id) ON DELETE SET NULL,
    document_id         uuid REFERENCES content_documents(id) ON DELETE SET NULL,
    link_id             uuid REFERENCES content_links(id) ON DELETE SET NULL,
    live_session_id     uuid REFERENCES live_sessions(id) ON DELETE SET NULL,
    is_preview          boolean NOT NULL DEFAULT false,
    display_order       integer NOT NULL DEFAULT 0,
    available_after_days integer NOT NULL DEFAULT 0 CHECK (available_after_days >= 0),
    available_at        timestamptz,
    status              publish_status NOT NULL DEFAULT 'draft',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz,
    CONSTRAINT course_lessons_body_matches_kind CHECK (
        (content_kind = 'video'        AND video_id IS NOT NULL) OR
        (content_kind = 'document'     AND document_id IS NOT NULL) OR
        (content_kind = 'link'         AND link_id IS NOT NULL) OR
        (content_kind = 'live_session' AND live_session_id IS NOT NULL) OR
        (content_kind IN ('quiz','assignment'))
    )
);
CREATE INDEX idx_course_lessons_course ON course_lessons (course_id, display_order);
CREATE INDEX idx_course_lessons_section ON course_lessons (section_id, display_order);
CREATE INDEX idx_course_lessons_tenant ON course_lessons (tenant_id);
CREATE INDEX idx_course_lessons_video ON course_lessons (video_id);
CREATE INDEX idx_course_lessons_document ON course_lessons (document_id);
CREATE INDEX idx_course_lessons_live_session ON course_lessons (live_session_id);
SELECT apply_tenant_table('course_lessons');

-- ──────────────────────────────────────────────────────────── qr_check_ins
CREATE TABLE qr_check_ins (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    session_id uuid NOT NULL REFERENCES live_sessions(id) ON DELETE CASCADE,
    code       text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_qr_check_ins_tenant ON qr_check_ins (tenant_id);
CREATE INDEX idx_qr_check_ins_session ON qr_check_ins (session_id);
SELECT apply_tenant_table('qr_check_ins');

-- ────────────────────────────────────────────────────────── content_progress
CREATE TABLE content_progress (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lesson_id    uuid NOT NULL REFERENCES course_lessons(id) ON DELETE CASCADE,
    watched_sec  integer NOT NULL DEFAULT 0,
    position_sec integer NOT NULL DEFAULT 0,
    completed_at timestamptz,
    last_at      timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, lesson_id)
);
CREATE INDEX idx_content_progress_tenant ON content_progress (tenant_id);
CREATE INDEX idx_content_progress_lesson ON content_progress (lesson_id);
SELECT apply_tenant_table('content_progress');

-- ────────────────────────────────────────────────────────── lesson_bookmarks
CREATE TABLE lesson_bookmarks (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lesson_id    uuid NOT NULL REFERENCES course_lessons(id) ON DELETE CASCADE,
    position_sec integer NOT NULL DEFAULT 0,
    note         text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_lesson_bookmarks_tenant ON lesson_bookmarks (tenant_id);
CREATE INDEX idx_lesson_bookmarks_user ON lesson_bookmarks (user_id, lesson_id);
SELECT apply_tenant_table('lesson_bookmarks');

-- ─────────────────────────────────────────────────────────── certificates
CREATE TABLE certificates (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id  uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    serial     text NOT NULL UNIQUE,
    status     certificate_status NOT NULL DEFAULT 'issued',
    issued_at  timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    pdf_key    text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, course_id)
);
CREATE INDEX idx_certificates_tenant ON certificates (tenant_id);
CREATE INDEX idx_certificates_course ON certificates (course_id);
SELECT apply_tenant_table('certificates');

-- ────────────────────────────────────────────────────────── session_messages
-- Partitioned monthly; retained ~90d by a scheduleworker job. PK includes
-- the partition key.
CREATE TABLE session_messages (
    id         uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    session_id uuid NOT NULL REFERENCES live_sessions(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       text NOT NULL DEFAULT 'chat' CHECK (kind IN ('chat','system','pinned')),
    body       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
CREATE TABLE session_messages_default PARTITION OF session_messages DEFAULT;
CREATE INDEX idx_session_messages_session ON session_messages (session_id, created_at DESC);
CREATE INDEX idx_session_messages_tenant ON session_messages (tenant_id);
SELECT apply_tenant_table('session_messages');
