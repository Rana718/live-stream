-- taxonomy.sql — exam_categories (platform) + subjects/chapters/topics (tenant).

-- name: ListExamCategories :many
SELECT id, parent_id, name, slug, description, icon_url, display_order
FROM exam_categories WHERE is_active ORDER BY display_order, name;

-- name: GetExamCategory :one
SELECT id, parent_id, name, slug, description, icon_url FROM exam_categories WHERE id = $1;

-- name: CreateExamCategory :one
INSERT INTO exam_categories (parent_id, name, slug, description, icon_url, display_order)
VALUES (sqlc.narg(parent_id)::uuid, $1, sqlc.arg(slug)::citext,
        sqlc.narg(description)::text, sqlc.narg(icon_url)::text,
        COALESCE(sqlc.narg(display_order)::int, 0))
RETURNING id, parent_id, name, slug, description, icon_url, display_order;

-- name: UpdateExamCategory :one
UPDATE exam_categories SET
    name = COALESCE(sqlc.narg(name)::text, name),
    description = COALESCE(sqlc.narg(description)::text, description),
    icon_url = COALESCE(sqlc.narg(icon_url)::text, icon_url),
    display_order = COALESCE(sqlc.narg(display_order)::int, display_order),
    is_active = COALESCE(sqlc.narg(is_active)::boolean, is_active)
WHERE id = $1
RETURNING id, name, slug, description, icon_url, display_order, is_active;

-- name: DeleteExamCategory :exec
UPDATE exam_categories SET is_active = false WHERE id = $1;

-- ─────────────────────────────────────────────────────────────── subjects

-- name: ListSubjects :many
SELECT id, exam_category_id, name, code, icon_url, display_order
FROM subjects WHERE tenant_id = $1 AND deleted_at IS NULL
ORDER BY display_order, name;

-- name: GetSubject :one
SELECT id, tenant_id, exam_category_id, name, code, icon_url, display_order
FROM subjects WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateSubject :one
INSERT INTO subjects (tenant_id, exam_category_id, name, code, icon_url, display_order)
VALUES ($1, sqlc.narg(exam_category_id)::uuid, $2, sqlc.narg(code)::text,
        sqlc.narg(icon_url)::text, COALESCE(sqlc.narg(display_order)::int, 0))
RETURNING id, tenant_id, exam_category_id, name, code, icon_url, display_order;

-- name: UpdateSubject :one
UPDATE subjects SET
    name = COALESCE(sqlc.narg(name)::text, name),
    code = COALESCE(sqlc.narg(code)::text, code),
    icon_url = COALESCE(sqlc.narg(icon_url)::text, icon_url),
    exam_category_id = COALESCE(sqlc.narg(exam_category_id)::uuid, exam_category_id),
    display_order = COALESCE(sqlc.narg(display_order)::int, display_order)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, name, code, icon_url, exam_category_id, display_order;

-- name: DeleteSubject :exec
UPDATE subjects SET deleted_at = now() WHERE id = $1;

-- ─────────────────────────────────────────────────────────────── chapters

-- name: ListChaptersBySubject :many
SELECT id, subject_id, name, description, display_order, is_free
FROM chapters WHERE tenant_id = $1 AND subject_id = $2 AND deleted_at IS NULL
ORDER BY display_order;

-- name: GetChapter :one
SELECT id, tenant_id, subject_id, name, description, display_order, is_free
FROM chapters WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateChapter :one
INSERT INTO chapters (tenant_id, subject_id, name, description, display_order, is_free)
VALUES ($1, $2, $3, sqlc.narg(description)::text,
        COALESCE(sqlc.narg(display_order)::int, 0),
        COALESCE(sqlc.narg(is_free)::boolean, false))
RETURNING id, tenant_id, subject_id, name, description, display_order, is_free;

-- name: UpdateChapter :one
UPDATE chapters SET
    name = COALESCE(sqlc.narg(name)::text, name),
    description = COALESCE(sqlc.narg(description)::text, description),
    display_order = COALESCE(sqlc.narg(display_order)::int, display_order),
    is_free = COALESCE(sqlc.narg(is_free)::boolean, is_free)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, subject_id, name, description, display_order, is_free;

-- name: DeleteChapter :exec
UPDATE chapters SET deleted_at = now() WHERE id = $1;

-- ───────────────────────────────────────────────────────────────── topics

-- name: ListTopicsByChapter :many
SELECT id, chapter_id, name, description, display_order, is_free
FROM topics WHERE tenant_id = $1 AND chapter_id = $2 AND deleted_at IS NULL
ORDER BY display_order;

-- name: GetTopic :one
SELECT id, tenant_id, chapter_id, name, description, display_order, is_free
FROM topics WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateTopic :one
INSERT INTO topics (tenant_id, chapter_id, name, description, display_order, is_free)
VALUES ($1, $2, $3, sqlc.narg(description)::text,
        COALESCE(sqlc.narg(display_order)::int, 0),
        COALESCE(sqlc.narg(is_free)::boolean, false))
RETURNING id, tenant_id, chapter_id, name, description, display_order, is_free;

-- name: UpdateTopic :one
UPDATE topics SET
    name = COALESCE(sqlc.narg(name)::text, name),
    description = COALESCE(sqlc.narg(description)::text, description),
    display_order = COALESCE(sqlc.narg(display_order)::int, display_order),
    is_free = COALESCE(sqlc.narg(is_free)::boolean, is_free)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, chapter_id, name, description, display_order, is_free;

-- name: DeleteTopic :exec
UPDATE topics SET deleted_at = now() WHERE id = $1;
