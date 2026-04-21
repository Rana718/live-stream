-- name: CreateAssignment :one
INSERT INTO assignments (batch_id, course_id, chapter_id, topic_id, title, description,
                         attachment_url, due_date, max_marks, is_published, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetAssignmentByID :one
SELECT * FROM assignments WHERE id = $1 LIMIT 1;

-- name: ListAssignmentsByBatch :many
SELECT * FROM assignments
WHERE batch_id = $1 AND is_published = TRUE
ORDER BY due_date ASC NULLS LAST, created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAssignmentsByCourse :many
SELECT * FROM assignments
WHERE course_id = $1 AND is_published = TRUE
ORDER BY due_date ASC NULLS LAST, created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAssignmentsCreatedBy :many
SELECT * FROM assignments WHERE created_by = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: UpdateAssignment :one
UPDATE assignments
SET title = $2, description = $3, attachment_url = $4, due_date = $5,
    max_marks = $6, is_published = $7, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteAssignment :exec
DELETE FROM assignments WHERE id = $1;

-- name: SubmitAssignment :one
INSERT INTO assignment_submissions (assignment_id, user_id, submission_text, file_path, status)
VALUES ($1, $2, $3, $4, 'submitted')
ON CONFLICT (assignment_id, user_id) DO UPDATE
    SET submission_text = EXCLUDED.submission_text,
        file_path = COALESCE(EXCLUDED.file_path, assignment_submissions.file_path),
        submitted_at = CURRENT_TIMESTAMP,
        status = 'submitted',
        updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetSubmissionByID :one
SELECT * FROM assignment_submissions WHERE id = $1 LIMIT 1;

-- name: GetMySubmission :one
SELECT * FROM assignment_submissions
WHERE assignment_id = $1 AND user_id = $2
LIMIT 1;

-- name: ListSubmissionsForAssignment :many
SELECT s.*, u.email, u.full_name
FROM assignment_submissions s
JOIN users u ON u.id = s.user_id
WHERE s.assignment_id = $1
ORDER BY s.submitted_at DESC
LIMIT $2 OFFSET $3;

-- name: ListMySubmissions :many
SELECT s.*, a.title AS assignment_title, a.due_date, a.max_marks
FROM assignment_submissions s
JOIN assignments a ON a.id = s.assignment_id
WHERE s.user_id = $1
ORDER BY s.submitted_at DESC
LIMIT $2 OFFSET $3;

-- name: GradeSubmission :one
UPDATE assignment_submissions
SET marks_obtained = $2, feedback = $3, graded_by = $4,
    graded_at = CURRENT_TIMESTAMP, status = 'graded', updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;
