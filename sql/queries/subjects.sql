-- name: CreateSubject :one
-- tenant_id derived from the parent course (NOT NULL FK).
INSERT INTO subjects (course_id, name, description, icon_url, display_order, tenant_id)
VALUES ($1, $2, $3, $4, $5, (SELECT tenant_id FROM courses WHERE id = $1))
RETURNING *;

-- name: GetSubjectByID :one
SELECT * FROM subjects WHERE id = $1 LIMIT 1;

-- name: ListSubjectsByCourse :many
SELECT * FROM subjects WHERE course_id = $1 ORDER BY display_order ASC, name ASC;

-- name: UpdateSubject :one
UPDATE subjects
SET name = $2, description = $3, icon_url = $4, display_order = $5, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteSubject :exec
DELETE FROM subjects WHERE id = $1;
