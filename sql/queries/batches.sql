-- name: CreateBatch :one
INSERT INTO batches (course_id, name, description, instructor_id, start_date, end_date, max_students, is_active)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetBatchByID :one
SELECT * FROM batches WHERE id = $1 LIMIT 1;

-- name: ListBatchesByCourse :many
SELECT * FROM batches
WHERE course_id = $1 AND is_active = TRUE
ORDER BY start_date ASC NULLS LAST;

-- name: ListBatchesByInstructor :many
SELECT * FROM batches
WHERE instructor_id = $1
ORDER BY created_at DESC;

-- name: UpdateBatch :one
UPDATE batches
SET name = $2, description = $3, instructor_id = $4, start_date = $5, end_date = $6,
    max_students = $7, is_active = $8, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: IncrementBatchStudents :exec
UPDATE batches SET current_students = current_students + 1 WHERE id = $1;

-- name: DecrementBatchStudents :exec
UPDATE batches SET current_students = GREATEST(current_students - 1, 0) WHERE id = $1;

-- name: DeleteBatch :exec
DELETE FROM batches WHERE id = $1;
