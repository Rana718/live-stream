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
