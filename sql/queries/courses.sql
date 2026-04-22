-- name: CreateCourse :one
INSERT INTO courses (exam_category_id, title, slug, description, thumbnail_url, price, discounted_price, duration_months, language, level, is_free, is_published, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetCourseByID :one
SELECT * FROM courses WHERE id = $1 LIMIT 1;

-- name: GetCourseBySlug :one
SELECT * FROM courses WHERE slug = $1 LIMIT 1;

-- name: ListPublishedCourses :many
SELECT * FROM courses
WHERE is_published = TRUE
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListCoursesByExamCategory :many
SELECT * FROM courses
WHERE exam_category_id = $1 AND is_published = TRUE
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListCoursesByLanguage :many
SELECT * FROM courses
WHERE language = $1 AND is_published = TRUE
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: SearchCourses :many
SELECT * FROM courses
WHERE is_published = TRUE
  AND (search_vector @@ plainto_tsquery('simple', $1) OR title ILIKE '%' || $1 || '%')
ORDER BY ts_rank(search_vector, plainto_tsquery('simple', $1)) DESC
LIMIT $2 OFFSET $3;

-- name: UpdateCourse :one
UPDATE courses
SET title = $2, description = $3, thumbnail_url = $4, price = $5, discounted_price = $6,
    duration_months = $7, language = $8, level = $9, is_free = $10, is_published = $11,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteCourse :exec
DELETE FROM courses WHERE id = $1;

-- name: CountCoursesByExamCategory :one
SELECT COUNT(*) FROM courses WHERE exam_category_id = $1 AND is_published = TRUE;

-- name: ListCoursesForLearner :many
-- Personalized feed: a course is shown if its class_level and exam_goal tags
-- are either NULL (universal) or match the learner's onboarding selection.
-- Pass '' (empty string) to opt out of a dimension.
SELECT * FROM courses
WHERE is_published = TRUE
  AND (class_level IS NULL OR $1::text = '' OR class_level = $1)
  AND (exam_goal   IS NULL OR $2::text = '' OR exam_goal   = $2)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;
