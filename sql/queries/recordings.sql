-- name: CreateRecording :one
-- tenant_id is derived from the parent stream (NOT NULL FK) rather than
-- passed explicitly — guarantees it can never drift from the stream's own
-- tenant, and avoids threading tenantID through every recording-creation
-- call site (some of which fire from RTMP callbacks, not request handlers).
INSERT INTO recordings (stream_id, file_path, status, tenant_id)
VALUES ($1, $2, $3, (SELECT tenant_id FROM streams WHERE id = $1))
RETURNING *;

-- name: GetRecordingByID :one
SELECT * FROM recordings WHERE id = $1 LIMIT 1;

-- name: GetRecordingsByStreamID :many
SELECT * FROM recordings WHERE stream_id = $1;

-- name: UpdateRecordingStatus :one
UPDATE recordings
SET status = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: UpdateRecordingDetails :one
UPDATE recordings
SET file_size = $2, duration = $3, thumbnail_url = $4, status = $5, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteRecording :exec
DELETE FROM recordings WHERE id = $1;
