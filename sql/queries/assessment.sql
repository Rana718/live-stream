-- assessment.sql — question bank, tests assembled from it, attempts, and
-- append-only responses (latest row per question is the answer).

-- ─────────────────────────────────────────────────────────── question_bank

-- name: CreateQuestion :one
INSERT INTO question_bank (
    tenant_id, subject_id, topic_id, kind, stem_rich, solution_rich, image_url,
    difficulty, default_marks, negative_marks, numeric_answer, numeric_tolerance,
    tags, status, created_by
)
VALUES ($1, sqlc.narg(subject_id)::uuid, sqlc.narg(topic_id)::uuid, $2,
        COALESCE(sqlc.narg(stem_rich)::jsonb, '{}'::jsonb),
        COALESCE(sqlc.narg(solution_rich)::jsonb, '{}'::jsonb),
        sqlc.narg(image_url)::text,
        COALESCE(sqlc.narg(difficulty)::text, 'medium'),
        COALESCE(sqlc.narg(default_marks)::numeric, 1),
        COALESCE(sqlc.narg(negative_marks)::numeric, 0),
        sqlc.narg(numeric_answer)::numeric, sqlc.narg(numeric_tolerance)::numeric,
        COALESCE(sqlc.narg(tags)::text[], '{}'),
        COALESCE(sqlc.narg(status)::publish_status, 'published'),
        sqlc.narg(created_by)::uuid)
RETURNING id, tenant_id, kind, difficulty, default_marks, negative_marks, status;

-- name: GetQuestion :one
SELECT id, tenant_id, subject_id, topic_id, kind, stem_rich, solution_rich,
       image_url, difficulty, default_marks, negative_marks, numeric_answer,
       numeric_tolerance, tags, status
FROM question_bank WHERE id = $1 AND deleted_at IS NULL;

-- name: ListQuestions :many
SELECT id, kind, difficulty, default_marks, tags, status
FROM question_bank
WHERE tenant_id = $1 AND deleted_at IS NULL
  AND (sqlc.narg(topic_id)::uuid IS NULL OR topic_id = sqlc.narg(topic_id)::uuid)
  AND (sqlc.narg(difficulty)::text IS NULL OR difficulty = sqlc.narg(difficulty)::text)
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: DeleteQuestion :exec
UPDATE question_bank SET deleted_at = now() WHERE id = $1;

-- name: AddQuestionOption :one
INSERT INTO question_options (tenant_id, question_id, label, body_rich, image_url, is_correct, display_order)
VALUES ($1, $2, sqlc.narg(label)::text, COALESCE(sqlc.narg(body_rich)::jsonb, '{}'::jsonb),
        sqlc.narg(image_url)::text, COALESCE(sqlc.narg(is_correct)::boolean, false),
        COALESCE(sqlc.narg(display_order)::int, 0))
RETURNING id, question_id, label, is_correct, display_order;

-- name: ListQuestionOptions :many
SELECT id, label, body_rich, image_url, is_correct, display_order
FROM question_options WHERE question_id = $1 ORDER BY display_order;

-- name: ListCorrectOptionIDs :many
SELECT id FROM question_options WHERE question_id = $1 AND is_correct;

-- name: DeleteQuestionOptions :exec
DELETE FROM question_options WHERE question_id = $1;

-- ───────────────────────────────────────────────────────────────── tests

-- name: CreateTest :one
INSERT INTO tests (
    tenant_id, course_id, subject_id, chapter_id, topic_id, exam_category_id,
    title, description, kind, exam_year, duration_min, total_marks, pass_marks,
    negative_marking, shuffle_questions, max_tab_switches, attempts_allowed,
    language, is_free, status, available_from, available_until, created_by
)
VALUES ($1, sqlc.narg(course_id)::uuid, sqlc.narg(subject_id)::uuid,
        sqlc.narg(chapter_id)::uuid, sqlc.narg(topic_id)::uuid,
        sqlc.narg(exam_category_id)::uuid, $2, sqlc.narg(description)::text,
        COALESCE(sqlc.narg(kind)::test_kind, 'chapter_test'), sqlc.narg(exam_year)::int,
        COALESCE(sqlc.narg(duration_min)::int, 0),
        COALESCE(sqlc.narg(total_marks)::numeric, 0),
        COALESCE(sqlc.narg(pass_marks)::numeric, 0),
        COALESCE(sqlc.narg(negative_marking)::boolean, false),
        COALESCE(sqlc.narg(shuffle_questions)::boolean, false),
        COALESCE(sqlc.narg(max_tab_switches)::int, 0),
        COALESCE(sqlc.narg(attempts_allowed)::int, 1),
        COALESCE(sqlc.narg(language)::text, 'en'),
        COALESCE(sqlc.narg(is_free)::boolean, false),
        COALESCE(sqlc.narg(status)::publish_status, 'draft'),
        sqlc.narg(available_from)::timestamptz, sqlc.narg(available_until)::timestamptz,
        sqlc.narg(created_by)::uuid)
RETURNING id, tenant_id, title, kind, status, total_marks, attempts_allowed;

-- name: GetTest :one
SELECT id, tenant_id, course_id, subject_id, chapter_id, topic_id, title,
       description, kind, exam_year, duration_min, total_marks, pass_marks,
       negative_marking, shuffle_questions, max_tab_switches, attempts_allowed,
       is_free, status, available_from, available_until
FROM tests WHERE id = $1 AND deleted_at IS NULL;

-- name: ListTests :many
SELECT id, title, kind, duration_min, total_marks, is_free, status, available_from, available_until
FROM tests
WHERE tenant_id = $1 AND deleted_at IS NULL
  AND (sqlc.narg(course_id)::uuid IS NULL OR course_id = sqlc.narg(course_id)::uuid)
  AND (sqlc.narg(published_only)::boolean IS NOT TRUE OR status = 'published')
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: SetTestStatus :exec
UPDATE tests SET status = $2 WHERE id = $1;

-- name: DeleteTest :exec
UPDATE tests SET deleted_at = now() WHERE id = $1;

-- name: AddTestSection :one
INSERT INTO test_sections (tenant_id, test_id, title, display_order, marks_per_q, negative_per_q)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(display_order)::int, 0),
        sqlc.narg(marks_per_q)::numeric, sqlc.narg(negative_per_q)::numeric)
RETURNING id, test_id, title, display_order;

-- name: AddTestQuestion :exec
INSERT INTO test_questions (tenant_id, test_id, section_id, question_id, display_order, marks, negative)
VALUES ($1, $2, sqlc.narg(section_id)::uuid, $3, COALESCE(sqlc.narg(display_order)::int, 0),
        COALESCE(sqlc.narg(marks)::numeric, 1), COALESCE(sqlc.narg(negative)::numeric, 0))
ON CONFLICT (test_id, question_id) DO UPDATE SET
    marks = EXCLUDED.marks, negative = EXCLUDED.negative, display_order = EXCLUDED.display_order;

-- name: RemoveTestQuestion :exec
DELETE FROM test_questions WHERE test_id = $1 AND question_id = $2;

-- name: ListTestQuestions :many
SELECT tq.question_id, tq.section_id, tq.display_order, tq.marks, tq.negative,
       q.kind, q.stem_rich, q.image_url, q.difficulty
FROM test_questions tq JOIN question_bank q ON q.id = tq.question_id
WHERE tq.test_id = $1 ORDER BY tq.display_order;

-- name: CountTestQuestions :one
SELECT count(*), COALESCE(sum(marks), 0)::numeric AS total_marks
FROM test_questions WHERE test_id = $1;

-- ───────────────────────────────────────────────────────────── test_attempts

-- name: NextAttemptNumber :one
SELECT COALESCE(max(attempt_no), 0) + 1 FROM test_attempts WHERE test_id = $1 AND user_id = $2;

-- name: CreateTestAttempt :one
INSERT INTO test_attempts (tenant_id, test_id, user_id, attempt_no, max_score, question_snapshot)
VALUES ($1, $2, $3, $4, $5, COALESCE(sqlc.narg(question_snapshot)::jsonb, '[]'::jsonb))
RETURNING id, tenant_id, test_id, user_id, attempt_no, status, max_score, started_at;

-- name: GetTestAttempt :one
SELECT id, tenant_id, test_id, user_id, attempt_no, status, score, max_score,
       correct_count, wrong_count, skipped_count, duration_sec, question_snapshot,
       proctoring_events, started_at, submitted_at, graded_at
FROM test_attempts WHERE id = $1;

-- name: GetTestAttemptForUpdate :one
SELECT id, tenant_id, test_id, user_id, status, max_score
FROM test_attempts WHERE id = $1 FOR UPDATE;

-- name: AppendProctoringEvent :exec
UPDATE test_attempts SET proctoring_events = proctoring_events || sqlc.arg(event)::jsonb
WHERE id = $1;

-- name: FinalizeTestAttempt :one
UPDATE test_attempts
SET status = 'graded', score = $2, correct_count = $3, wrong_count = $4,
    skipped_count = $5, duration_sec = $6, submitted_at = now(), graded_at = now()
WHERE id = $1 AND status = 'in_progress'
RETURNING id, test_id, user_id, score, max_score, correct_count, wrong_count, skipped_count;

-- name: ExpireStaleAttempts :exec
UPDATE test_attempts SET status = 'expired'
WHERE status = 'in_progress' AND started_at < $1;

-- name: ListAttemptsForUser :many
SELECT a.id, a.test_id, a.attempt_no, a.status, a.score, a.max_score,
       a.correct_count, a.wrong_count, a.submitted_at, t.title
FROM test_attempts a JOIN tests t ON t.id = a.test_id
WHERE a.tenant_id = $1 AND a.user_id = $2
ORDER BY a.started_at DESC LIMIT $3 OFFSET $4;

-- ───────────────────────────────────────────────────────────── test_responses

-- name: SaveTestResponse :exec
INSERT INTO test_responses (
    tenant_id, attempt_id, question_id, selected_option_ids, numeric_answer,
    text_answer, is_correct, marks, time_sec
)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(selected_option_ids)::uuid[], '{}'),
        sqlc.narg(numeric_answer)::numeric, sqlc.narg(text_answer)::text,
        sqlc.narg(is_correct)::boolean, COALESCE(sqlc.narg(marks)::numeric, 0),
        COALESCE(sqlc.narg(time_sec)::int, 0));

-- name: ListLatestResponses :many
-- One row per question — the most recent answer in the attempt.
SELECT DISTINCT ON (question_id)
    question_id, selected_option_ids, numeric_answer, text_answer, is_correct, marks, time_sec
FROM test_responses
WHERE attempt_id = $1
ORDER BY question_id, created_at DESC;

-- name: ScoreAttemptResponses :one
-- Aggregate the latest answer per question for final scoring.
WITH latest AS (
    SELECT DISTINCT ON (question_id) question_id, is_correct, marks
    FROM test_responses WHERE attempt_id = $1
    ORDER BY question_id, created_at DESC
)
SELECT
    COALESCE(sum(marks), 0)::numeric               AS score,
    count(*) FILTER (WHERE is_correct IS TRUE)     AS correct_count,
    count(*) FILTER (WHERE is_correct IS FALSE)    AS wrong_count
FROM latest;
