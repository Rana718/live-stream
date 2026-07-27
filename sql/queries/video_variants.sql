-- name: CreateVideoVariant :one
-- recording_id/lecture_id are both nullable — no guaranteed parent to
-- derive from, passed explicitly.
INSERT INTO video_variants (recording_id, lecture_id, quality, file_path, file_size, bitrate_kbps, width, height, codec, tenant_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetVideoVariantByID :one
SELECT * FROM video_variants WHERE id = $1 LIMIT 1;

-- name: ListVariantsByRecording :many
SELECT * FROM video_variants WHERE recording_id = $1 ORDER BY bitrate_kbps DESC;

-- name: ListVariantsByLecture :many
SELECT * FROM video_variants WHERE lecture_id = $1 ORDER BY bitrate_kbps DESC;

-- name: DeleteVideoVariant :exec
DELETE FROM video_variants WHERE id = $1;

-- name: CreateDownloadToken :one
-- tenant_id derived from the requesting user (NOT NULL FK).
INSERT INTO download_tokens (user_id, resource_type, resource_id, token, expires_at, tenant_id)
VALUES ($1, $2, $3, $4, $5, (SELECT tenant_id FROM users WHERE id = $1))
RETURNING *;

-- name: GetDownloadTokenByToken :one
SELECT * FROM download_tokens WHERE token = $1 AND used = FALSE AND expires_at > CURRENT_TIMESTAMP LIMIT 1;

-- name: MarkDownloadTokenUsed :exec
UPDATE download_tokens SET used = TRUE WHERE id = $1;

-- name: PurgeExpiredDownloadTokens :exec
DELETE FROM download_tokens WHERE expires_at < CURRENT_TIMESTAMP;
