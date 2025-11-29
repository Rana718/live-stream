-- name: CreateStream :one
INSERT INTO streams (title, description, instructor_id, stream_key, scheduled_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetStreamByID :one
SELECT * FROM streams WHERE id = $1 LIMIT 1;

-- name: GetStreamByKey :one
SELECT * FROM streams WHERE stream_key = $1 LIMIT 1;

-- name: UpdateStreamStatus :one
UPDATE streams
SET status = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: StartStream :one
UPDATE streams
SET status = 'live', started_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: EndStream :one
UPDATE streams
SET status = 'ended', ended_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: UpdateViewerCount :exec
UPDATE streams
SET viewer_count = $2
WHERE id = $1;

-- name: ListStreams :many
SELECT * FROM streams
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListLiveStreams :many
SELECT * FROM streams
WHERE status = 'live'
ORDER BY started_at DESC;

-- name: DeleteStream :exec
DELETE FROM streams WHERE id = $1;
