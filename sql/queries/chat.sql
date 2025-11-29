-- name: CreateChatMessage :one
INSERT INTO chat_messages (stream_id, user_id, message)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetChatMessagesByStreamID :many
SELECT cm.*, u.username, u.full_name
FROM chat_messages cm
JOIN users u ON cm.user_id = u.id
WHERE cm.stream_id = $1
ORDER BY cm.created_at DESC
LIMIT $2 OFFSET $3;

-- name: DeleteChatMessage :exec
DELETE FROM chat_messages WHERE id = $1;
