-- name: UpsertAttendance :one
INSERT INTO attendance (user_id, lecture_id, batch_id, status, join_time, leave_time,
                        watched_seconds, is_auto, marked_by, notes, geo_lat, geo_lng, qr_code)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (user_id, lecture_id) DO UPDATE
    SET status = EXCLUDED.status,
        join_time = COALESCE(EXCLUDED.join_time, attendance.join_time),
        leave_time = COALESCE(EXCLUDED.leave_time, attendance.leave_time),
        watched_seconds = GREATEST(attendance.watched_seconds, EXCLUDED.watched_seconds),
        is_auto = EXCLUDED.is_auto,
        marked_by = COALESCE(EXCLUDED.marked_by, attendance.marked_by),
        notes = COALESCE(EXCLUDED.notes, attendance.notes),
        updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetAttendance :one
SELECT * FROM attendance WHERE user_id = $1 AND lecture_id = $2 LIMIT 1;

-- name: ListAttendanceByLecture :many
SELECT a.*, u.full_name, u.email
FROM attendance a
JOIN users u ON u.id = a.user_id
WHERE a.lecture_id = $1
ORDER BY a.status ASC, u.full_name ASC;

-- name: ListMyAttendance :many
SELECT a.*, l.title AS lecture_title, l.scheduled_at
FROM attendance a
JOIN lectures l ON l.id = a.lecture_id
WHERE a.user_id = $1
ORDER BY a.created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAttendanceByBatch :many
SELECT a.*, u.full_name, u.email, l.title AS lecture_title
FROM attendance a
JOIN users u ON u.id = a.user_id
JOIN lectures l ON l.id = a.lecture_id
WHERE a.batch_id = $1
ORDER BY a.created_at DESC
LIMIT $2 OFFSET $3;

-- name: AttendancePercentByUser :one
SELECT
    COUNT(*)::bigint                                        AS total_classes,
    COUNT(*) FILTER (WHERE status IN ('present','late'))::bigint AS attended,
    ROUND(
      COUNT(*) FILTER (WHERE status IN ('present','late'))::numeric
        / NULLIF(COUNT(*),0) * 100, 2
    )                                                       AS percent
FROM attendance
WHERE user_id = $1;

-- name: AttendancePercentByUserSubject :many
SELECT l.subject_id,
       COUNT(*)::bigint                                        AS total_classes,
       COUNT(*) FILTER (WHERE a.status IN ('present','late'))::bigint AS attended,
       ROUND(
         COUNT(*) FILTER (WHERE a.status IN ('present','late'))::numeric
           / NULLIF(COUNT(*),0) * 100, 2
       )                                                       AS percent
FROM attendance a
JOIN lectures l ON l.id = a.lecture_id
WHERE a.user_id = $1
GROUP BY l.subject_id;

-- name: AttendanceMonthlyReport :many
SELECT DATE(a.created_at) AS day,
       COUNT(*)::bigint   AS total,
       COUNT(*) FILTER (WHERE status = 'present')::bigint AS present,
       COUNT(*) FILTER (WHERE status = 'late')::bigint    AS late,
       COUNT(*) FILTER (WHERE status = 'absent')::bigint  AS absent
FROM attendance a
WHERE a.user_id = $1
  AND a.created_at >= date_trunc('month', $2::timestamp)
  AND a.created_at <  date_trunc('month', $2::timestamp) + interval '1 month'
GROUP BY DATE(a.created_at)
ORDER BY day;

-- name: ExportAttendanceByBatch :many
SELECT u.email, u.full_name, l.title AS lecture_title, l.scheduled_at,
       a.status, a.join_time, a.leave_time, a.watched_seconds
FROM attendance a
JOIN users u ON u.id = a.user_id
JOIN lectures l ON l.id = a.lecture_id
WHERE a.batch_id = $1
  AND a.created_at >= $2
  AND a.created_at <  $3
ORDER BY a.created_at DESC;

-- name: LowAttendanceUsers :many
SELECT a.user_id, u.email, u.full_name,
       COUNT(*)::bigint AS total,
       COUNT(*) FILTER (WHERE a.status IN ('present','late'))::bigint AS attended,
       ROUND(
         COUNT(*) FILTER (WHERE a.status IN ('present','late'))::numeric
           / NULLIF(COUNT(*),0) * 100, 2
       ) AS percent
FROM attendance a
JOIN users u ON u.id = a.user_id
WHERE ($1::uuid IS NULL OR a.batch_id = $1)
GROUP BY a.user_id, u.email, u.full_name
HAVING ROUND(
  COUNT(*) FILTER (WHERE a.status IN ('present','late'))::numeric
    / NULLIF(COUNT(*),0) * 100, 2
) < $2
ORDER BY percent ASC;

-- name: CreateQRCode :one
INSERT INTO class_qr_codes (lecture_id, code, expires_at, created_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetQRCode :one
SELECT * FROM class_qr_codes
WHERE code = $1 AND expires_at > CURRENT_TIMESTAMP
LIMIT 1;

-- name: PurgeExpiredQRCodes :exec
DELETE FROM class_qr_codes WHERE expires_at < CURRENT_TIMESTAMP;
