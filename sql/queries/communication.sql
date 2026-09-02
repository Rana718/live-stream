-- communication.sql — notifications, deliveries, preferences, announcements,
-- device tokens, WhatsApp threads.

-- name: CreateNotification :one
INSERT INTO notifications (tenant_id, user_id, template_key, title, body, data, entity_type, entity_id)
VALUES ($1, $2, $3, $4, sqlc.narg(body)::text, COALESCE(sqlc.narg(data)::jsonb, '{}'::jsonb),
        sqlc.narg(entity_type)::text, sqlc.narg(entity_id)::uuid)
RETURNING id, tenant_id, user_id, template_key, title, body, data, created_at;

-- name: ListNotifications :many
SELECT id, template_key, title, body, data, entity_type, entity_id, read_at, created_at
FROM notifications WHERE tenant_id = $1 AND user_id = $2
ORDER BY created_at DESC LIMIT $3 OFFSET $4;

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE tenant_id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkNotificationRead :exec
UPDATE notifications SET read_at = now() WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = now() WHERE tenant_id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: DeleteNotification :exec
DELETE FROM notifications WHERE id = $1 AND user_id = $2;

-- name: PruneOldNotifications :exec
DELETE FROM notifications WHERE created_at < $1;

-- name: RecordDelivery :one
INSERT INTO notification_deliveries (tenant_id, notification_id, channel, status)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(status)::delivery_status, 'queued'))
RETURNING id, notification_id, channel, status;

-- name: UpdateDeliveryStatus :exec
UPDATE notification_deliveries
SET status = $2, provider_message_id = sqlc.narg(provider_message_id)::text,
    error = sqlc.narg(error)::text,
    sent_at = CASE WHEN $2 = 'sent'::delivery_status THEN now() ELSE sent_at END,
    delivered_at = CASE WHEN $2 = 'delivered'::delivery_status THEN now() ELSE delivered_at END
WHERE id = $1;

-- name: GetNotificationPreference :one
SELECT enabled FROM notification_preferences
WHERE tenant_id = $1 AND user_id = $2 AND channel = $3 AND category = $4;

-- name: SetNotificationPreference :exec
INSERT INTO notification_preferences (tenant_id, user_id, channel, category, enabled)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, user_id, channel, category) DO UPDATE SET enabled = EXCLUDED.enabled;

-- name: CreateAnnouncement :one
INSERT INTO announcements (tenant_id, created_by, course_id, batch_id, title, body, audience, priority, scheduled_at, expires_at)
VALUES ($1, sqlc.narg(created_by)::uuid, sqlc.narg(course_id)::uuid, sqlc.narg(batch_id)::uuid,
        $2, $3, COALESCE(sqlc.narg(audience)::jsonb, '{}'::jsonb),
        COALESCE(sqlc.narg(priority)::text, 'normal'),
        sqlc.narg(scheduled_at)::timestamptz, sqlc.narg(expires_at)::timestamptz)
RETURNING id, title, body, audience, priority, published_at;

-- name: ListAnnouncements :many
SELECT id, course_id, batch_id, title, body, priority, published_at, expires_at
FROM announcements
WHERE tenant_id = $1 AND published_at <= now() AND (expires_at IS NULL OR expires_at > now())
  AND (sqlc.narg(course_id)::uuid IS NULL OR course_id = sqlc.narg(course_id)::uuid)
  AND (sqlc.narg(batch_id)::uuid IS NULL OR batch_id = sqlc.narg(batch_id)::uuid)
ORDER BY published_at DESC LIMIT $2 OFFSET $3;

-- name: DeleteAnnouncement :exec
DELETE FROM announcements WHERE id = $1;

-- name: RegisterDeviceToken :exec
INSERT INTO device_tokens (tenant_id, user_id, token, platform)
VALUES ($1, $2, $3, $4)
ON CONFLICT (token) DO UPDATE SET user_id = EXCLUDED.user_id, tenant_id = EXCLUDED.tenant_id,
    last_seen_at = now(), revoked_at = NULL;

-- name: RevokeDeviceToken :exec
UPDATE device_tokens SET revoked_at = now() WHERE token = $1;

-- name: ListActiveDeviceTokens :many
SELECT token, platform FROM device_tokens
WHERE tenant_id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: UpsertMessagingThread :one
INSERT INTO messaging_threads (tenant_id, user_id, channel, phone, last_message_at)
VALUES ($1, sqlc.narg(user_id)::uuid, $2, sqlc.arg(phone)::citext, now())
ON CONFLICT (tenant_id, channel, phone) DO UPDATE SET
    last_message_at = now(),
    user_id = COALESCE(EXCLUDED.user_id, messaging_threads.user_id)
RETURNING id, tenant_id, user_id, channel, phone, unread_count;

-- name: AddMessagingMessage :one
INSERT INTO messaging_messages (tenant_id, thread_id, direction, body, provider_id, status)
VALUES ($1, $2, $3, $4, sqlc.narg(provider_id)::text, COALESCE(sqlc.narg(status)::delivery_status, 'queued'))
RETURNING id, thread_id, direction, body, status, created_at;

-- name: IncrementThreadUnread :exec
UPDATE messaging_threads SET unread_count = unread_count + 1 WHERE id = $1;

-- name: MarkThreadRead :exec
UPDATE messaging_threads SET unread_count = 0 WHERE id = $1;

-- name: ListMessagingMessages :many
SELECT id, direction, body, status, created_at
FROM messaging_messages WHERE thread_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;
