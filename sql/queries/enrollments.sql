-- name: CreateEnrollment :one
INSERT INTO enrollments (tenant_id, user_id, course_id, batch_id, status)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, course_id) DO UPDATE SET status = EXCLUDED.status, enrolled_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetEnrollment :one
SELECT * FROM enrollments WHERE user_id = $1 AND course_id = $2 LIMIT 1;

-- name: GetEnrollmentByID :one
SELECT * FROM enrollments WHERE id = $1 LIMIT 1;

-- name: ListUserEnrollments :many
SELECT e.*, c.title AS course_title, c.thumbnail_url AS course_thumbnail
FROM enrollments e
JOIN courses c ON c.id = e.course_id
WHERE e.user_id = $1
ORDER BY e.enrolled_at DESC;

-- name: ListCourseEnrollments :many
SELECT e.*, u.email, u.full_name
FROM enrollments e
JOIN users u ON u.id = e.user_id
WHERE e.course_id = $1
ORDER BY e.enrolled_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateEnrollmentProgress :exec
UPDATE enrollments
SET progress_percent = $2,
    status = CASE WHEN $2 >= 100 THEN 'completed' ELSE status END,
    completed_at = CASE WHEN $2 >= 100 THEN CURRENT_TIMESTAMP ELSE completed_at END
WHERE id = $1;

-- name: CancelEnrollment :exec
UPDATE enrollments SET status = 'cancelled' WHERE user_id = $1 AND course_id = $2;
