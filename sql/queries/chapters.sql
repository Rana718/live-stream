-- name: CreateChapter :one
INSERT INTO chapters (subject_id, name, description, display_order, is_free)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetChapterByID :one
SELECT * FROM chapters WHERE id = $1 LIMIT 1;

-- name: ListChaptersBySubject :many
SELECT * FROM chapters WHERE subject_id = $1 ORDER BY display_order ASC, name ASC;

-- name: UpdateChapter :one
UPDATE chapters
SET name = $2, description = $3, display_order = $4, is_free = $5, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteChapter :exec
DELETE FROM chapters WHERE id = $1;
