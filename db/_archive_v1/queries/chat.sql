-- name: CreateChatMessage :one
-- tenant_id derived from the parent stream (NOT NULL FK) — see the same
-- note on CreateRecording in recordings.sql.
INSERT INTO chat_messages (stream_id, user_id, message, tenant_id)
VALUES ($1, $2, $3, (SELECT tenant_id FROM streams WHERE id = $1))
RETURNING *;

-- name: GetChatMessagesByStreamID :many
SELECT cm.*, u.full_name, u.phone_number
FROM chat_messages cm
JOIN users u ON cm.user_id = u.id
WHERE cm.stream_id = $1
ORDER BY cm.created_at DESC
LIMIT $2 OFFSET $3;

-- name: DeleteChatMessage :exec
DELETE FROM chat_messages WHERE id = $1;
