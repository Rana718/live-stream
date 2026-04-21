-- name: CreateVideoVariant :one
INSERT INTO video_variants (recording_id, lecture_id, quality, file_path, file_size, bitrate_kbps, width, height, codec)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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
INSERT INTO download_tokens (user_id, resource_type, resource_id, token, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetDownloadTokenByToken :one
SELECT * FROM download_tokens WHERE token = $1 AND used = FALSE AND expires_at > CURRENT_TIMESTAMP LIMIT 1;

-- name: MarkDownloadTokenUsed :exec
UPDATE download_tokens SET used = TRUE WHERE id = $1;

-- name: PurgeExpiredDownloadTokens :exec
DELETE FROM download_tokens WHERE expires_at < CURRENT_TIMESTAMP;
