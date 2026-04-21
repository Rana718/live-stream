-- name: CreateTestAttempt :one
INSERT INTO test_attempts (user_id, test_id, total_questions, status)
VALUES ($1, $2, $3, 'in_progress')
RETURNING *;

-- name: GetTestAttemptByID :one
SELECT * FROM test_attempts WHERE id = $1 LIMIT 1;

-- name: GetActiveAttempt :one
SELECT * FROM test_attempts
WHERE user_id = $1 AND test_id = $2 AND status = 'in_progress'
ORDER BY started_at DESC
LIMIT 1;

-- name: ListUserAttempts :many
SELECT ta.*, t.title AS test_title, t.test_type
FROM test_attempts ta
JOIN tests t ON t.id = ta.test_id
WHERE ta.user_id = $1
ORDER BY ta.started_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAttemptsByTest :many
SELECT * FROM test_attempts WHERE test_id = $1 ORDER BY score DESC NULLS LAST LIMIT $2 OFFSET $3;

-- name: SubmitTestAttempt :one
UPDATE test_attempts
SET status = 'completed', submitted_at = CURRENT_TIMESTAMP,
    score = $2, correct_count = $3, wrong_count = $4, skipped_count = $5, time_taken_seconds = $6
WHERE id = $1
RETURNING *;

-- name: AbandonTestAttempt :exec
UPDATE test_attempts SET status = 'abandoned' WHERE id = $1;

-- name: UpsertTestAnswer :one
INSERT INTO test_answers (attempt_id, question_id, selected_option_id, numerical_answer,
                          subjective_answer, is_correct, marks_obtained, time_taken_seconds)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (attempt_id, question_id) DO UPDATE
    SET selected_option_id = EXCLUDED.selected_option_id,
        numerical_answer = EXCLUDED.numerical_answer,
        subjective_answer = EXCLUDED.subjective_answer,
        is_correct = EXCLUDED.is_correct,
        marks_obtained = EXCLUDED.marks_obtained,
        time_taken_seconds = EXCLUDED.time_taken_seconds,
        answered_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: ListAnswersByAttempt :many
SELECT * FROM test_answers WHERE attempt_id = $1 ORDER BY answered_at ASC;

-- name: GetAnswerForQuestion :one
SELECT * FROM test_answers WHERE attempt_id = $1 AND question_id = $2 LIMIT 1;

-- name: CountCorrectAnswers :one
SELECT COUNT(*) FROM test_answers WHERE attempt_id = $1 AND is_correct = TRUE;

-- name: CountWrongAnswers :one
SELECT COUNT(*) FROM test_answers WHERE attempt_id = $1 AND is_correct = FALSE;

-- name: SumMarksForAttempt :one
SELECT COALESCE(SUM(marks_obtained), 0)::numeric AS total_marks FROM test_answers WHERE attempt_id = $1;
