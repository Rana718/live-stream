-- name: AdminDashboardStats :one
SELECT
    (SELECT COUNT(*) FROM users WHERE role = 'student')::bigint     AS total_students,
    (SELECT COUNT(*) FROM users WHERE role = 'instructor')::bigint  AS total_instructors,
    (SELECT COUNT(*) FROM users)::bigint                            AS total_users,
    (SELECT COUNT(*) FROM courses WHERE is_published = TRUE)::bigint AS total_courses,
    (SELECT COUNT(*) FROM courses WHERE approval_status = 'pending')::bigint AS pending_approval,
    (SELECT COUNT(*) FROM batches WHERE is_active = TRUE)::bigint    AS active_batches,
    (SELECT COUNT(*) FROM enrollments WHERE status = 'active')::bigint AS active_enrollments,
    (SELECT COUNT(*) FROM streams WHERE status = 'live')::bigint     AS live_streams,
    (SELECT COUNT(*) FROM tests WHERE is_published = TRUE)::bigint   AS total_tests,
    (SELECT COUNT(*) FROM test_attempts WHERE status = 'completed')::bigint AS total_attempts,
    (SELECT COALESCE(SUM(amount),0)::numeric FROM payments WHERE status = 'captured') AS total_revenue_captured;

-- name: AttendanceAggregateByBatch :many
SELECT a.batch_id,
       COUNT(*)::bigint AS total,
       COUNT(*) FILTER (WHERE a.status IN ('present','late'))::bigint AS attended,
       ROUND(
         COUNT(*) FILTER (WHERE a.status IN ('present','late'))::numeric
           / NULLIF(COUNT(*),0) * 100, 2
       ) AS attendance_percent
FROM attendance a
WHERE a.batch_id IS NOT NULL
GROUP BY a.batch_id
ORDER BY attendance_percent ASC NULLS LAST;

-- name: AdminListAllUsers :many
SELECT u.*, COUNT(DISTINCT e.id)::bigint AS enrolled_courses
FROM users u
LEFT JOIN enrollments e ON e.user_id = u.id AND e.status = 'active'
WHERE ($1::text = '' OR u.role = $1)
GROUP BY u.id
ORDER BY u.created_at DESC
LIMIT $2 OFFSET $3;

-- name: ApproveCourse :one
UPDATE courses
SET approval_status = 'approved',
    approved_by = $2,
    approved_at = CURRENT_TIMESTAMP,
    is_published = TRUE,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: RejectCourse :one
UPDATE courses
SET approval_status = 'rejected',
    approved_by = $2,
    approved_at = CURRENT_TIMESTAMP,
    is_published = FALSE,
    rejection_reason = $3,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: ListPendingApprovalCourses :many
SELECT * FROM courses WHERE approval_status = 'pending' ORDER BY created_at ASC LIMIT $1 OFFSET $2;
