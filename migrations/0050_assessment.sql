-- 0050_assessment.sql
-- Reusable question bank + tests assembled from it + attempts + responses.
-- Marks/scores are numeric(n,2) — academic values, not money.
--
-- test_responses is APPEND-ONLY: a student changing an answer writes a new
-- row; the latest row per (attempt_id, question_id) is the answer. This
-- lets the table be partitioned + time-retained without an upsert fighting
-- the partition-key-in-unique-constraint rule.

-- ─────────────────────────────────────────────────────────── question_bank
CREATE TABLE question_bank (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    subject_id        uuid REFERENCES subjects(id) ON DELETE SET NULL,
    topic_id          uuid REFERENCES topics(id) ON DELETE SET NULL,
    kind              question_kind NOT NULL DEFAULT 'mcq_single',
    stem_rich         jsonb NOT NULL DEFAULT '{}'::jsonb,
    solution_rich     jsonb NOT NULL DEFAULT '{}'::jsonb,
    image_url         text,
    difficulty        text NOT NULL DEFAULT 'medium' CHECK (difficulty IN ('easy','medium','hard')),
    default_marks     numeric(6,2) NOT NULL DEFAULT 1,
    negative_marks    numeric(6,2) NOT NULL DEFAULT 0,
    numeric_answer    numeric(14,4),
    numeric_tolerance numeric(14,4),
    tags              text[] NOT NULL DEFAULT '{}',
    status            publish_status NOT NULL DEFAULT 'published',
    created_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    deleted_at        timestamptz
);
CREATE INDEX idx_question_bank_tenant ON question_bank (tenant_id);
CREATE INDEX idx_question_bank_topic ON question_bank (topic_id);
CREATE INDEX idx_question_bank_subject ON question_bank (subject_id);
CREATE INDEX idx_question_bank_tags ON question_bank USING gin (tags);
SELECT apply_tenant_table('question_bank');

-- ────────────────────────────────────────────────────────── question_options
CREATE TABLE question_options (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    question_id   uuid NOT NULL REFERENCES question_bank(id) ON DELETE CASCADE,
    label         text,
    body_rich     jsonb NOT NULL DEFAULT '{}'::jsonb,
    image_url     text,
    is_correct    boolean NOT NULL DEFAULT false,
    display_order integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_question_options_question ON question_options (question_id, display_order);
CREATE INDEX idx_question_options_tenant ON question_options (tenant_id);
SELECT apply_tenant_table('question_options');

-- ───────────────────────────────────────────────────────────────── tests
CREATE TABLE tests (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id        uuid REFERENCES courses(id) ON DELETE SET NULL,
    subject_id       uuid REFERENCES subjects(id) ON DELETE SET NULL,
    chapter_id       uuid REFERENCES chapters(id) ON DELETE SET NULL,
    topic_id         uuid REFERENCES topics(id) ON DELETE SET NULL,
    exam_category_id uuid REFERENCES exam_categories(id) ON DELETE SET NULL,
    title            text NOT NULL,
    description      text,
    kind             test_kind NOT NULL DEFAULT 'chapter_test',
    exam_year        integer,
    duration_min     integer NOT NULL DEFAULT 0,
    total_marks      numeric(8,2) NOT NULL DEFAULT 0,
    pass_marks       numeric(8,2) NOT NULL DEFAULT 0,
    negative_marking boolean NOT NULL DEFAULT false,
    shuffle_questions boolean NOT NULL DEFAULT false,
    max_tab_switches integer NOT NULL DEFAULT 0,
    attempts_allowed integer NOT NULL DEFAULT 1,
    language         text NOT NULL DEFAULT 'en',
    is_free          boolean NOT NULL DEFAULT false,
    status           publish_status NOT NULL DEFAULT 'draft',
    available_from   timestamptz,
    available_until  timestamptz,
    created_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz
);
CREATE INDEX idx_tests_tenant_status ON tests (tenant_id, status);
CREATE INDEX idx_tests_course ON tests (course_id);
CREATE INDEX idx_tests_kind ON tests (kind);
SELECT apply_tenant_table('tests');

-- ────────────────────────────────────────────────────────────── test_sections
CREATE TABLE test_sections (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    test_id        uuid NOT NULL REFERENCES tests(id) ON DELETE CASCADE,
    title          text NOT NULL,
    display_order  integer NOT NULL DEFAULT 0,
    marks_per_q    numeric(6,2),
    negative_per_q numeric(6,2),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_test_sections_test ON test_sections (test_id, display_order);
CREATE INDEX idx_test_sections_tenant ON test_sections (tenant_id);
SELECT apply_tenant_table('test_sections');

-- ────────────────────────────────────────────────────────────── test_questions
CREATE TABLE test_questions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    test_id       uuid NOT NULL REFERENCES tests(id) ON DELETE CASCADE,
    section_id    uuid REFERENCES test_sections(id) ON DELETE SET NULL,
    question_id   uuid NOT NULL REFERENCES question_bank(id) ON DELETE RESTRICT,
    display_order integer NOT NULL DEFAULT 0,
    marks         numeric(6,2) NOT NULL DEFAULT 1,
    negative      numeric(6,2) NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (test_id, question_id)
);
CREATE INDEX idx_test_questions_test ON test_questions (test_id, display_order);
CREATE INDEX idx_test_questions_question ON test_questions (question_id);
CREATE INDEX idx_test_questions_section ON test_questions (section_id);
CREATE INDEX idx_test_questions_tenant ON test_questions (tenant_id);
SELECT apply_tenant_table('test_questions');

-- ────────────────────────────────────────────────────────────── test_attempts
CREATE TABLE test_attempts (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    test_id           uuid NOT NULL REFERENCES tests(id) ON DELETE CASCADE,
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    attempt_no        integer NOT NULL DEFAULT 1,
    status            attempt_status NOT NULL DEFAULT 'in_progress',
    score             numeric(8,2) NOT NULL DEFAULT 0,
    max_score         numeric(8,2) NOT NULL DEFAULT 0,
    correct_count     integer NOT NULL DEFAULT 0,
    wrong_count       integer NOT NULL DEFAULT 0,
    skipped_count     integer NOT NULL DEFAULT 0,
    duration_sec      integer NOT NULL DEFAULT 0,
    question_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb,
    proctoring_events jsonb NOT NULL DEFAULT '[]'::jsonb,
    started_at        timestamptz NOT NULL DEFAULT now(),
    submitted_at      timestamptz,
    graded_at         timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (test_id, user_id, attempt_no)
);
CREATE INDEX idx_test_attempts_tenant_user ON test_attempts (tenant_id, user_id);
CREATE INDEX idx_test_attempts_test ON test_attempts (test_id);
CREATE INDEX idx_test_attempts_status ON test_attempts (status);
SELECT apply_tenant_table('test_attempts');

-- ────────────────────────────────────────────────────────────── test_responses
-- Append-only, partitioned monthly. Latest row per (attempt_id,
-- question_id) is the student's answer.
CREATE TABLE test_responses (
    id                  uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    attempt_id          uuid NOT NULL REFERENCES test_attempts(id) ON DELETE CASCADE,
    question_id         uuid NOT NULL REFERENCES question_bank(id) ON DELETE RESTRICT,
    selected_option_ids uuid[] NOT NULL DEFAULT '{}',
    numeric_answer      numeric(14,4),
    text_answer         text,
    is_correct          boolean,
    marks               numeric(6,2) NOT NULL DEFAULT 0,
    time_sec            integer NOT NULL DEFAULT 0,
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
CREATE TABLE test_responses_default PARTITION OF test_responses DEFAULT;
CREATE INDEX idx_test_responses_attempt ON test_responses (attempt_id, question_id, created_at DESC);
CREATE INDEX idx_test_responses_tenant ON test_responses (tenant_id);
SELECT apply_tenant_table('test_responses');
