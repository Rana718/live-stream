-- name: CreateExamCategory :one
INSERT INTO exam_categories (name, slug, description, icon_url, display_order)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetExamCategoryByID :one
SELECT * FROM exam_categories WHERE id = $1 LIMIT 1;

-- name: GetExamCategoryBySlug :one
SELECT * FROM exam_categories WHERE slug = $1 LIMIT 1;

-- name: ListExamCategories :many
SELECT * FROM exam_categories
WHERE is_active = TRUE
ORDER BY display_order ASC, name ASC;

-- name: UpdateExamCategory :one
UPDATE exam_categories
SET name = $2, description = $3, icon_url = $4, display_order = $5, is_active = $6, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteExamCategory :exec
DELETE FROM exam_categories WHERE id = $1;
