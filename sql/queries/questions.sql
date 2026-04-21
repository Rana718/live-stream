-- name: CreateQuestion :one
INSERT INTO questions (test_id, topic_id, question_text, question_type, image_url, marks,
                      negative_marks, difficulty, explanation, correct_numerical_answer, display_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetQuestionByID :one
SELECT * FROM questions WHERE id = $1 LIMIT 1;

-- name: ListQuestionsByTest :many
SELECT * FROM questions WHERE test_id = $1 ORDER BY display_order ASC, created_at ASC;

-- name: CountQuestionsByTest :one
SELECT COUNT(*) FROM questions WHERE test_id = $1;

-- name: UpdateQuestion :one
UPDATE questions
SET question_text = $2, question_type = $3, image_url = $4, marks = $5, negative_marks = $6,
    difficulty = $7, explanation = $8, correct_numerical_answer = $9, display_order = $10,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteQuestion :exec
DELETE FROM questions WHERE id = $1;

-- name: CreateQuestionOption :one
INSERT INTO question_options (question_id, option_text, image_url, is_correct, display_order)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListOptionsByQuestion :many
SELECT * FROM question_options WHERE question_id = $1 ORDER BY display_order ASC;

-- name: GetQuestionOptionByID :one
SELECT * FROM question_options WHERE id = $1 LIMIT 1;

-- name: DeleteQuestionOption :exec
DELETE FROM question_options WHERE id = $1;

-- name: DeleteOptionsByQuestion :exec
DELETE FROM question_options WHERE question_id = $1;
