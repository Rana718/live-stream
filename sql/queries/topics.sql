-- name: CreateTopic :one
INSERT INTO topics (chapter_id, name, description, display_order, is_free)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTopicByID :one
SELECT * FROM topics WHERE id = $1 LIMIT 1;

-- name: ListTopicsByChapter :many
SELECT * FROM topics WHERE chapter_id = $1 ORDER BY display_order ASC, name ASC;

-- name: UpdateTopic :one
UPDATE topics
SET name = $2, description = $3, display_order = $4, is_free = $5, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteTopic :exec
DELETE FROM topics WHERE id = $1;
