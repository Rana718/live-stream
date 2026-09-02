-- 0055_learning_activities.sql
-- Attendance, assignments + submissions, and doubts + answers. Depends on
-- 0040 (live_sessions, batches, courses, course_lessons) and 0030 (topics).

-- ─────────────────────────────────────────────────────────────── attendance
CREATE TABLE attendance (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id   uuid REFERENCES live_sessions(id) ON DELETE CASCADE,
    batch_id     uuid REFERENCES batches(id) ON DELETE SET NULL,
    status       attendance_status NOT NULL DEFAULT 'absent',
    join_time    timestamptz,
    leave_time   timestamptz,
    watched_sec  integer NOT NULL DEFAULT 0,
    is_auto      boolean NOT NULL DEFAULT false,
    method       text NOT NULL DEFAULT 'manual' CHECK (method IN ('manual','auto','qr','geo')),
    marked_by    uuid REFERENCES users(id) ON DELETE SET NULL,
    geo_lat      double precision,
    geo_lng      double precision,
    notes        text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, session_id)
);
CREATE INDEX idx_attendance_tenant_user ON attendance (tenant_id, user_id);
CREATE INDEX idx_attendance_session ON attendance (session_id);
CREATE INDEX idx_attendance_batch ON attendance (batch_id);
SELECT apply_tenant_table('attendance');

-- ────────────────────────────────────────────────────────────── assignments
CREATE TABLE assignments (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id      uuid REFERENCES courses(id) ON DELETE CASCADE,
    batch_id       uuid REFERENCES batches(id) ON DELETE CASCADE,
    lesson_id      uuid REFERENCES course_lessons(id) ON DELETE SET NULL,
    chapter_id     uuid REFERENCES chapters(id) ON DELETE SET NULL,
    topic_id       uuid REFERENCES topics(id) ON DELETE SET NULL,
    title          text NOT NULL,
    description    text,
    attachment_url text,
    due_at         timestamptz,
    max_marks      numeric(8,2) NOT NULL DEFAULT 100,
    status         publish_status NOT NULL DEFAULT 'draft',
    created_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    deleted_at     timestamptz
);
CREATE INDEX idx_assignments_tenant ON assignments (tenant_id);
CREATE INDEX idx_assignments_course ON assignments (course_id);
CREATE INDEX idx_assignments_batch ON assignments (batch_id);
CREATE INDEX idx_assignments_due ON assignments (due_at);
SELECT apply_tenant_table('assignments');

CREATE TABLE assignment_submissions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    assignment_id   uuid NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    submission_text text,
    file_key        text,
    submitted_at    timestamptz NOT NULL DEFAULT now(),
    graded_at       timestamptz,
    marks_obtained  numeric(8,2),
    feedback        text,
    graded_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    status          text NOT NULL DEFAULT 'submitted'
                        CHECK (status IN ('submitted','graded','returned')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (assignment_id, user_id)
);
CREATE INDEX idx_assignment_submissions_assignment ON assignment_submissions (assignment_id);
CREATE INDEX idx_assignment_submissions_user ON assignment_submissions (user_id);
CREATE INDEX idx_assignment_submissions_graded_by ON assignment_submissions (graded_by);
CREATE INDEX idx_assignment_submissions_tenant ON assignment_submissions (tenant_id);
SELECT apply_tenant_table('assignment_submissions');

-- ─────────────────────────────────────────────────────────────────── doubts
CREATE TABLE doubts (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lesson_id      uuid REFERENCES course_lessons(id) ON DELETE SET NULL,
    chapter_id     uuid REFERENCES chapters(id) ON DELETE SET NULL,
    topic_id       uuid REFERENCES topics(id) ON DELETE SET NULL,
    question_text  text NOT NULL,
    input_type     text NOT NULL DEFAULT 'text' CHECK (input_type IN ('text','voice','image')),
    attachment_url text,
    status         doubt_status NOT NULL DEFAULT 'open',
    language       text NOT NULL DEFAULT 'en',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_doubts_tenant_user ON doubts (tenant_id, user_id);
CREATE INDEX idx_doubts_status ON doubts (status);
CREATE INDEX idx_doubts_lesson ON doubts (lesson_id);
SELECT apply_tenant_table('doubts');

CREATE TABLE doubt_answers (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    doubt_id    uuid NOT NULL REFERENCES doubts(id) ON DELETE CASCADE,
    answer_text text NOT NULL,
    answer_type text NOT NULL DEFAULT 'ai' CHECK (answer_type IN ('ai','instructor')),
    answered_by uuid REFERENCES users(id) ON DELETE SET NULL,
    is_accepted boolean NOT NULL DEFAULT false,
    model_name  text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_doubt_answers_doubt ON doubt_answers (doubt_id);
CREATE INDEX idx_doubt_answers_tenant ON doubt_answers (tenant_id);
CREATE INDEX idx_doubt_answers_answered_by ON doubt_answers (answered_by);
SELECT apply_tenant_table('doubt_answers');
