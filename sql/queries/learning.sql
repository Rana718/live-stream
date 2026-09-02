-- learning.sql — attendance, assignments + submissions, doubts + answers.

-- ────────────────────────────────────────────────────────────── attendance

-- name: UpsertAttendance :one
INSERT INTO attendance (
    tenant_id, user_id, session_id, batch_id, status, join_time, leave_time,
    watched_sec, is_auto, method, marked_by, geo_lat, geo_lng, notes
)
VALUES ($1, $2, $3, sqlc.narg(batch_id)::uuid, $4,
        sqlc.narg(join_time)::timestamptz, sqlc.narg(leave_time)::timestamptz,
        COALESCE(sqlc.narg(watched_sec)::int, 0),
        COALESCE(sqlc.narg(is_auto)::boolean, false),
        COALESCE(sqlc.narg(method)::text, 'manual'),
        sqlc.narg(marked_by)::uuid, sqlc.narg(geo_lat)::double precision,
        sqlc.narg(geo_lng)::double precision, sqlc.narg(notes)::text)
ON CONFLICT (user_id, session_id) DO UPDATE SET
    status = EXCLUDED.status,
    leave_time = COALESCE(EXCLUDED.leave_time, attendance.leave_time),
    watched_sec = GREATEST(attendance.watched_sec, EXCLUDED.watched_sec),
    method = EXCLUDED.method,
    marked_by = COALESCE(EXCLUDED.marked_by, attendance.marked_by)
RETURNING id, user_id, session_id, status, watched_sec, method;

-- name: BulkMarkAttendance :exec
INSERT INTO attendance (tenant_id, user_id, session_id, batch_id, status, is_auto, method, marked_by)
SELECT $1, unnest(sqlc.arg(user_ids)::uuid[]), $2, sqlc.narg(batch_id)::uuid, $3, false, 'manual', $4
ON CONFLICT (user_id, session_id) DO UPDATE SET status = EXCLUDED.status, marked_by = EXCLUDED.marked_by;

-- name: ListAttendanceBySession :many
SELECT a.user_id, a.status, a.join_time, a.leave_time, a.watched_sec, a.method,
       u.full_name, u.phone
FROM attendance a JOIN users u ON u.id = a.user_id
WHERE a.tenant_id = $1 AND a.session_id = $2
ORDER BY u.full_name;

-- name: ListMyAttendance :many
SELECT a.session_id, a.status, a.join_time, a.watched_sec, s.title, s.scheduled_start
FROM attendance a JOIN live_sessions s ON s.id = a.session_id
WHERE a.tenant_id = $1 AND a.user_id = $2
ORDER BY s.scheduled_start DESC NULLS LAST LIMIT $3 OFFSET $4;

-- name: AttendanceStatsForUser :one
SELECT
    count(*)                                              AS total,
    count(*) FILTER (WHERE status IN ('present','late'))  AS present
FROM attendance WHERE tenant_id = $1 AND user_id = $2
  AND (sqlc.narg(batch_id)::uuid IS NULL OR batch_id = sqlc.narg(batch_id)::uuid);

-- name: CreateQRCheckIn :one
INSERT INTO qr_check_ins (tenant_id, session_id, code, expires_at, created_by)
VALUES ($1, $2, $3, $4, sqlc.narg(created_by)::uuid)
RETURNING id, session_id, code, expires_at;

-- name: GetQRCheckIn :one
SELECT id, tenant_id, session_id, code, expires_at
FROM qr_check_ins WHERE code = $1;

-- ────────────────────────────────────────────────────────────── assignments

-- name: CreateAssignment :one
INSERT INTO assignments (
    tenant_id, course_id, batch_id, lesson_id, chapter_id, topic_id, title,
    description, attachment_url, due_at, max_marks, status, created_by
)
VALUES ($1, sqlc.narg(course_id)::uuid, sqlc.narg(batch_id)::uuid,
        sqlc.narg(lesson_id)::uuid, sqlc.narg(chapter_id)::uuid, sqlc.narg(topic_id)::uuid,
        $2, sqlc.narg(description)::text, sqlc.narg(attachment_url)::text,
        sqlc.narg(due_at)::timestamptz, COALESCE(sqlc.narg(max_marks)::numeric, 100),
        COALESCE(sqlc.narg(status)::publish_status, 'draft'), sqlc.narg(created_by)::uuid)
RETURNING id, tenant_id, title, due_at, max_marks, status;

-- name: GetAssignment :one
SELECT id, tenant_id, course_id, batch_id, lesson_id, title, description,
       attachment_url, due_at, max_marks, status
FROM assignments WHERE id = $1 AND deleted_at IS NULL;

-- name: ListAssignments :many
SELECT id, course_id, batch_id, title, description, attachment_url, due_at, max_marks, status
FROM assignments
WHERE tenant_id = $1 AND deleted_at IS NULL
  AND (sqlc.narg(course_id)::uuid IS NULL OR course_id = sqlc.narg(course_id)::uuid)
  AND (sqlc.narg(batch_id)::uuid IS NULL OR batch_id = sqlc.narg(batch_id)::uuid)
  AND (sqlc.narg(created_by)::uuid IS NULL OR created_by = sqlc.narg(created_by)::uuid)
ORDER BY due_at DESC NULLS LAST LIMIT $2 OFFSET $3;

-- name: ListMySubmissions :many
SELECT s.id, s.assignment_id, s.submission_text, s.file_key, s.submitted_at,
       s.marks_obtained, s.feedback, s.status, a.title AS assignment_title, a.max_marks
FROM assignment_submissions s
JOIN assignments a ON a.id = s.assignment_id
WHERE s.tenant_id = $1 AND s.user_id = $2
ORDER BY s.submitted_at DESC
LIMIT $3 OFFSET $4;

-- name: SetAssignmentStatus :exec
UPDATE assignments SET status = $2 WHERE id = $1;

-- name: UpdateAssignment :one
UPDATE assignments SET
    title          = COALESCE(sqlc.narg(title)::text, title),
    description    = COALESCE(sqlc.narg(description)::text, description),
    attachment_url = COALESCE(sqlc.narg(attachment_url)::text, attachment_url),
    due_at         = COALESCE(sqlc.narg(due_at)::timestamptz, due_at),
    max_marks      = COALESCE(sqlc.narg(max_marks)::numeric, max_marks),
    status         = COALESCE(sqlc.narg(status)::publish_status, status)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, tenant_id, title, description, attachment_url, due_at, max_marks, status;

-- name: DeleteAssignment :exec
UPDATE assignments SET deleted_at = now() WHERE id = $1;

-- name: SubmitAssignment :one
INSERT INTO assignment_submissions (tenant_id, assignment_id, user_id, submission_text, file_key, submitted_at)
VALUES ($1, $2, $3, sqlc.narg(submission_text)::text, sqlc.narg(file_key)::text, now())
ON CONFLICT (assignment_id, user_id) DO UPDATE SET
    submission_text = EXCLUDED.submission_text,
    file_key = EXCLUDED.file_key,
    submitted_at = now(),
    status = 'submitted'
RETURNING id, assignment_id, user_id, status, submitted_at;

-- name: GradeSubmission :one
UPDATE assignment_submissions
SET marks_obtained = $2, feedback = sqlc.narg(feedback)::text, graded_by = $3,
    graded_at = now(), status = 'graded'
WHERE id = $1
RETURNING id, assignment_id, user_id, marks_obtained, status, graded_at;

-- name: GetMySubmission :one
SELECT id, submission_text, file_key, submitted_at, marks_obtained, feedback, status
FROM assignment_submissions WHERE assignment_id = $1 AND user_id = $2;

-- name: ListSubmissions :many
SELECT s.id, s.user_id, s.submitted_at, s.marks_obtained, s.status,
       u.full_name, u.phone
FROM assignment_submissions s JOIN users u ON u.id = s.user_id
WHERE s.assignment_id = $1 ORDER BY s.submitted_at DESC;

-- ─────────────────────────────────────────────────────────────────── doubts

-- name: CreateDoubt :one
INSERT INTO doubts (tenant_id, user_id, lesson_id, chapter_id, topic_id, question_text, input_type, attachment_url, language)
VALUES ($1, $2, sqlc.narg(lesson_id)::uuid, sqlc.narg(chapter_id)::uuid, sqlc.narg(topic_id)::uuid,
        $3, COALESCE(sqlc.narg(input_type)::text, 'text'), sqlc.narg(attachment_url)::text,
        COALESCE(sqlc.narg(language)::text, 'en'))
RETURNING id, tenant_id, user_id, question_text, status, created_at;

-- name: GetDoubt :one
SELECT id, tenant_id, user_id, lesson_id, chapter_id, topic_id, question_text,
       input_type, attachment_url, status, language, created_at
FROM doubts WHERE id = $1;

-- name: ListDoubtsForUser :many
SELECT id, question_text, status, created_at
FROM doubts WHERE tenant_id = $1 AND user_id = $2
ORDER BY created_at DESC LIMIT $3 OFFSET $4;

-- name: ListPendingDoubts :many
SELECT d.id, d.user_id, d.question_text, d.created_at, u.full_name
FROM doubts d JOIN users u ON u.id = d.user_id
WHERE d.tenant_id = $1 AND d.status = 'open'
ORDER BY d.created_at LIMIT $2 OFFSET $3;

-- name: ListAllDoubtsForTenant :many
SELECT d.id, d.user_id, d.question_text, d.status, d.created_at, u.full_name,
       (SELECT count(*) FROM doubt_answers a WHERE a.doubt_id = d.id) AS answers_count
FROM doubts d JOIN users u ON u.id = d.user_id
WHERE d.tenant_id = $1
ORDER BY d.created_at DESC LIMIT $2 OFFSET $3;

-- name: SetDoubtStatus :exec
UPDATE doubts SET status = $2 WHERE id = $1;

-- name: AddDoubtAnswer :one
INSERT INTO doubt_answers (tenant_id, doubt_id, answer_text, answer_type, answered_by, model_name)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(answer_type)::text, 'ai'),
        sqlc.narg(answered_by)::uuid, sqlc.narg(model_name)::text)
RETURNING id, doubt_id, answer_text, answer_type, answered_by, is_accepted, created_at;

-- name: ListDoubtAnswers :many
SELECT id, answer_text, answer_type, answered_by, is_accepted, model_name, created_at
FROM doubt_answers WHERE doubt_id = $1 ORDER BY created_at;

-- name: AcceptDoubtAnswer :exec
UPDATE doubt_answers SET is_accepted = true WHERE id = $1;
