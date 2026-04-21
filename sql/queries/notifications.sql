-- name: CreateNotification :one
INSERT INTO notifications (user_id, type, title, body, resource_type, resource_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListMyNotifications :many
SELECT * FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListMyUnreadNotifications :many
SELECT * FROM notifications
WHERE user_id = $1 AND is_read = FALSE
ORDER BY created_at DESC
LIMIT $2;

-- name: CountMyUnread :one
SELECT COUNT(*)::bigint FROM notifications WHERE user_id = $1 AND is_read = FALSE;

-- name: MarkNotificationRead :exec
UPDATE notifications
SET is_read = TRUE, read_at = CURRENT_TIMESTAMP
WHERE id = $1 AND user_id = $2;

-- name: MarkAllMyNotificationsRead :exec
UPDATE notifications
SET is_read = TRUE, read_at = CURRENT_TIMESTAMP
WHERE user_id = $1 AND is_read = FALSE;

-- name: DeleteNotification :exec
DELETE FROM notifications WHERE id = $1 AND user_id = $2;

-- name: CreateAnnouncement :one
INSERT INTO announcements (batch_id, course_id, created_by, title, body, priority, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetAnnouncementByID :one
SELECT * FROM announcements WHERE id = $1 LIMIT 1;

-- name: ListAnnouncementsGlobal :many
SELECT * FROM announcements
WHERE batch_id IS NULL AND course_id IS NULL
  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
ORDER BY published_at DESC
LIMIT $1 OFFSET $2;

-- name: ListAnnouncementsByBatch :many
SELECT * FROM announcements
WHERE batch_id = $1
  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
ORDER BY published_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAnnouncementsByCourse :many
SELECT * FROM announcements
WHERE course_id = $1
  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
ORDER BY published_at DESC
LIMIT $2 OFFSET $3;

-- name: DeleteAnnouncement :exec
DELETE FROM announcements WHERE id = $1;

-- name: FanOutToBatchEnrollees :exec
-- Fanout: create a notification for every active enrollee of a batch.
INSERT INTO notifications (user_id, type, title, body, resource_type, resource_id)
SELECT e.user_id, $1, $2, $3, 'announcement', $4
FROM enrollments e
WHERE e.batch_id = $5 AND e.status = 'active';

-- name: FanOutToCourseEnrollees :exec
INSERT INTO notifications (user_id, type, title, body, resource_type, resource_id)
SELECT e.user_id, $1, $2, $3, 'announcement', $4
FROM enrollments e
WHERE e.course_id = $5 AND e.status = 'active';
