-- analytics.sql — learner progress + tenant dashboards. schema-v2.
-- test_responses is append-only; "the answer" is the latest row per
-- (attempt_id, question_id).

-- name: UserAttemptStats :one
SELECT
    count(*)                                                     AS total_attempts,
    count(*) FILTER (WHERE status IN ('submitted','graded'))     AS completed_attempts,
    COALESCE(avg(score) FILTER (WHERE status IN ('submitted','graded')), 0)::numeric AS avg_score,
    COALESCE(max(score), 0)::numeric                             AS best_score,
    COALESCE(sum(duration_sec), 0)::bigint                       AS total_time_seconds
FROM test_attempts
WHERE tenant_id = $1 AND user_id = $2;

-- name: UserAvgTimePerQuestion :one
SELECT COALESCE(avg(r.time_sec), 0)::numeric
FROM test_responses r
JOIN test_attempts a ON a.id = r.attempt_id
WHERE a.tenant_id = $1 AND a.user_id = $2;

-- name: UserWatchedSeconds :one
SELECT COALESCE(sum(watched_sec), 0)::bigint
FROM content_progress WHERE tenant_id = $1 AND user_id = $2;

-- name: UserCompletedLessonCount :one
SELECT count(*)::bigint
FROM content_progress WHERE tenant_id = $1 AND user_id = $2 AND completed_at IS NOT NULL;

-- name: UserTopicAccuracy :many
WITH latest AS (
    SELECT DISTINCT ON (r.attempt_id, r.question_id)
        r.question_id, r.is_correct
    FROM test_responses r
    JOIN test_attempts a ON a.id = r.attempt_id
    WHERE a.tenant_id = $1 AND a.user_id = $2
    ORDER BY r.attempt_id, r.question_id, r.created_at DESC
)
SELECT qb.topic_id,
       count(*)::bigint                                    AS total_answers,
       count(*) FILTER (WHERE l.is_correct)::bigint        AS correct_answers,
       (100.0 * count(*) FILTER (WHERE l.is_correct) / NULLIF(count(*), 0))::float AS accuracy_percent
FROM latest l
JOIN question_bank qb ON qb.id = l.question_id
WHERE qb.topic_id IS NOT NULL
GROUP BY qb.topic_id
ORDER BY accuracy_percent ASC NULLS LAST;

-- name: UserDifficultyAccuracy :many
WITH latest AS (
    SELECT DISTINCT ON (r.attempt_id, r.question_id)
        r.question_id, r.is_correct
    FROM test_responses r
    JOIN test_attempts a ON a.id = r.attempt_id
    WHERE a.tenant_id = $1 AND a.user_id = $2
    ORDER BY r.attempt_id, r.question_id, r.created_at DESC
)
SELECT qb.difficulty,
       count(*)::bigint                             AS total_answers,
       count(*) FILTER (WHERE l.is_correct)::bigint AS correct_answers
FROM latest l
JOIN question_bank qb ON qb.id = l.question_id
GROUP BY qb.difficulty;

-- name: UserRecentAttempts :many
SELECT a.id, a.test_id, t.title AS test_title, a.score, a.correct_count,
       a.wrong_count, a.duration_sec, a.status, a.submitted_at
FROM test_attempts a
JOIN tests t ON t.id = a.test_id
WHERE a.tenant_id = $1 AND a.user_id = $2
ORDER BY a.started_at DESC
LIMIT $3;

-- name: TenantDashboardStats :one
SELECT
    (SELECT count(*) FROM courses WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND deleted_at IS NULL)                       AS total_courses,
    (SELECT count(*) FROM courses WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND status = 'published' AND deleted_at IS NULL) AS published_courses,
    (SELECT count(*) FROM tenant_users WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND role = 'student' AND deleted_at IS NULL) AS total_students,
    (SELECT count(*) FROM tenant_users WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND role = 'instructor' AND deleted_at IS NULL) AS total_instructors,
    (SELECT count(*) FROM enrollments WHERE tenant_id = sqlc.arg(tenant_id)::uuid)                                          AS total_enrollments,
    (SELECT COALESCE(sum(total_minor), 0)::bigint FROM orders WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND status = 'paid') AS revenue_minor,
    (SELECT count(*) FROM orders WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND status = 'paid')                           AS paid_orders;

-- name: TenantRevenueDaily :many
SELECT date_trunc('day', paid_at)::date AS day,
       COALESCE(sum(total_minor), 0)::bigint AS revenue_minor,
       count(*)::bigint AS orders
FROM orders
WHERE tenant_id = $1 AND status = 'paid' AND paid_at >= now() - interval '30 days'
GROUP BY 1 ORDER BY 1;

-- name: TenantTopCourses :many
SELECT p.course_id, c.title,
       count(*)::bigint AS sales,
       COALESCE(sum(oi.total_minor), 0)::bigint AS revenue_minor
FROM order_items oi
JOIN orders o ON o.id = oi.order_id AND o.status = 'paid'
JOIN products p ON p.id = oi.product_id AND p.course_id IS NOT NULL
JOIN courses c ON c.id = p.course_id
WHERE oi.tenant_id = $1
GROUP BY p.course_id, c.title
ORDER BY revenue_minor DESC
LIMIT $2;
