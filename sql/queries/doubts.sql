-- name: CreateDoubt :one
INSERT INTO doubts (user_id, lecture_id, chapter_id, topic_id, question_text, input_type, voice_url, language)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetDoubtByID :one
SELECT * FROM doubts WHERE id = $1 LIMIT 1;

-- name: ListUserDoubts :many
SELECT * FROM doubts WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: ListDoubtsByLecture :many
SELECT * FROM doubts WHERE lecture_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: ListPendingDoubts :many
SELECT * FROM doubts WHERE status = 'pending' ORDER BY created_at ASC LIMIT $1 OFFSET $2;

-- name: UpdateDoubtStatus :exec
UPDATE doubts SET status = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1;

-- name: DeleteDoubt :exec
DELETE FROM doubts WHERE id = $1;

-- name: CreateDoubtAnswer :one
INSERT INTO doubt_answers (doubt_id, answer_text, answer_type, answered_by, model_name)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListAnswersByDoubt :many
SELECT * FROM doubt_answers WHERE doubt_id = $1 ORDER BY created_at ASC;

-- name: AcceptDoubtAnswer :exec
UPDATE doubt_answers SET is_accepted = TRUE WHERE id = $1;
