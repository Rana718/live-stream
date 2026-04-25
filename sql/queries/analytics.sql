-- name: UserAttemptStats :one
SELECT
    COUNT(*)::bigint                                           AS total_attempts,
    COUNT(*) FILTER (WHERE status = 'completed')::bigint       AS completed_attempts,
    COALESCE(AVG(score) FILTER (WHERE status = 'completed'),0)::numeric AS avg_score,
    COALESCE(MAX(score),0)::numeric                            AS best_score,
    COALESCE(SUM(time_taken_seconds),0)::bigint                AS total_time_seconds
FROM test_attempts
WHERE user_id = $1;

-- name: UserTopicAccuracy :many
SELECT q.topic_id,
       COUNT(*)::bigint                               AS total_answers,
       COUNT(*) FILTER (WHERE ta.is_correct)::bigint  AS correct_answers,
       (COUNT(*) FILTER (WHERE ta.is_correct)::numeric
         / NULLIF(COUNT(*),0) * 100)                  AS accuracy_percent
FROM test_answers ta
JOIN questions q ON q.id = ta.question_id
JOIN test_attempts att ON att.id = ta.attempt_id
WHERE att.user_id = $1
GROUP BY q.topic_id
ORDER BY accuracy_percent ASC NULLS LAST;

-- name: UserDifficultyAccuracy :many
SELECT q.difficulty,
       COUNT(*)::bigint                               AS total_answers,
       COUNT(*) FILTER (WHERE ta.is_correct)::bigint  AS correct_answers
FROM test_answers ta
JOIN questions q ON q.id = ta.question_id
JOIN test_attempts att ON att.id = ta.attempt_id
WHERE att.user_id = $1
GROUP BY q.difficulty;

-- name: UserAvgTimePerQuestion :one
SELECT COALESCE(AVG(time_taken_seconds),0)::numeric AS avg_seconds
FROM test_answers ta
JOIN test_attempts att ON att.id = ta.attempt_id
WHERE att.user_id = $1;

-- name: UserRecentAttempts :many
SELECT ta.*, t.title AS test_title
FROM test_attempts ta
JOIN tests t ON t.id = ta.test_id
WHERE ta.user_id = $1 AND ta.status = 'completed'
ORDER BY ta.submitted_at DESC NULLS LAST
LIMIT $2;

-- name: UserWatchedSeconds :one
SELECT COALESCE(SUM(watched_seconds),0)::bigint AS total_seconds
FROM lecture_views WHERE user_id = $1;

-- name: UserCompletedLectureCount :one
SELECT COUNT(*)::bigint FROM lecture_views WHERE user_id = $1 AND completed = TRUE;

-- ─── Tenant-admin dashboard ─────────────────────────────────────────────
-- Tenant scoping comes from RLS — every count below is implicitly filtered
-- to the caller's tenant_id by the policies set up in migration 029.

-- name: TenantDashboardStats :one
SELECT
    (SELECT count(*) FROM users WHERE role = 'student') AS students,
    (SELECT count(*) FROM users WHERE role = 'instructor') AS instructors,
    (SELECT count(*) FROM courses) AS courses,
    (SELECT count(*) FROM lectures) AS lectures,
    (SELECT count(*) FROM enrollments) AS enrollments,
    (SELECT count(*) FROM streams WHERE status = 'live') AS live_now,
    (SELECT count(*) FROM streams) AS streams_total,
    (SELECT count(*) FROM test_attempts WHERE submitted_at > now() - interval '24 hours') AS attempts_24h,
    (SELECT COALESCE(sum(amount), 0) FROM payments WHERE status = 'paid' AND created_at > now() - interval '30 days') AS revenue_30d,
    (SELECT COALESCE(sum(amount), 0) FROM payments WHERE status = 'paid') AS revenue_alltime,
    (SELECT count(*) FROM users WHERE created_at > now() - interval '7 days') AS signups_7d,
    (SELECT count(*) FROM doubts WHERE status = 'pending') AS pending_doubts;

-- name: TenantRevenueDaily :many
SELECT date_trunc('day', created_at)::date AS day, COALESCE(sum(amount), 0)::bigint AS revenue
FROM payments
WHERE status = 'paid'
  AND created_at > now() - interval '30 days'
GROUP BY 1
ORDER BY 1 ASC;

-- name: TenantTopCourses :many
SELECT c.id, c.title, count(e.id) AS enrolled
FROM courses c
LEFT JOIN enrollments e ON e.course_id = c.id
GROUP BY c.id, c.title
ORDER BY enrolled DESC NULLS LAST
LIMIT $1;
