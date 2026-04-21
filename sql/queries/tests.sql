-- name: CreateTest :one
INSERT INTO tests (course_id, subject_id, chapter_id, topic_id, exam_category_id, title, description,
                  test_type, exam_year, duration_minutes, total_marks, passing_marks, negative_marking,
                  shuffle_questions, language, is_free, is_published, scheduled_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
RETURNING *;

-- name: GetTestByID :one
SELECT * FROM tests WHERE id = $1 LIMIT 1;

-- name: ListTestsByType :many
SELECT * FROM tests
WHERE test_type = $1 AND is_published = TRUE
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListTestsByChapter :many
SELECT * FROM tests WHERE chapter_id = $1 AND is_published = TRUE ORDER BY created_at DESC;

-- name: ListTestsBySubject :many
SELECT * FROM tests WHERE subject_id = $1 AND is_published = TRUE ORDER BY created_at DESC;

-- name: ListTestsByCourse :many
SELECT * FROM tests WHERE course_id = $1 AND is_published = TRUE ORDER BY created_at DESC;

-- name: ListPYQsByExamYear :many
SELECT * FROM tests
WHERE test_type = 'pyq' AND exam_year = $1 AND is_published = TRUE
ORDER BY created_at DESC;

-- name: ListPYQsByExamCategory :many
SELECT * FROM tests
WHERE test_type = 'pyq' AND exam_category_id = $1 AND is_published = TRUE
ORDER BY exam_year DESC NULLS LAST, created_at DESC;

-- name: UpdateTest :one
UPDATE tests
SET title = $2, description = $3, duration_minutes = $4, total_marks = $5, passing_marks = $6,
    negative_marking = $7, shuffle_questions = $8, language = $9, is_free = $10, is_published = $11,
    scheduled_at = $12, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteTest :exec
DELETE FROM tests WHERE id = $1;
