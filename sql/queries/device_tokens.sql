-- name: UpsertDeviceToken :one
-- A device token might already belong to a different user (logged out + new
-- user signed in on the same phone) — we re-key it to the new user.
INSERT INTO device_tokens (tenant_id, user_id, token, platform)
VALUES ($1, $2, $3, $4)
ON CONFLICT (token) DO UPDATE
    SET tenant_id    = EXCLUDED.tenant_id,
        user_id      = EXCLUDED.user_id,
        platform     = EXCLUDED.platform,
        last_seen_at = now()
RETURNING *;

-- name: ListDeviceTokensForUser :many
SELECT * FROM device_tokens WHERE user_id = $1 AND tenant_id = $2;

-- name: ListDeviceTokensForTenant :many
SELECT * FROM device_tokens WHERE tenant_id = $1
ORDER BY last_seen_at DESC LIMIT $2 OFFSET $3;

-- name: DeleteDeviceToken :exec
DELETE FROM device_tokens WHERE token = $1;

-- name: TouchDeviceToken :exec
UPDATE device_tokens SET last_seen_at = now() WHERE token = $1;

-- name: CountActiveDevicesForUser :one
-- "Active" = touched within the last 30 days. Anything older is treated
-- as evicted so a phone the user lost months ago doesn't keep its slot.
SELECT count(*) FROM device_tokens
WHERE tenant_id = $1
  AND user_id = $2
  AND last_seen_at > now() - interval '30 days';

-- name: EvictOldestDeviceForUser :exec
-- Drops the oldest device for a user when they exceed the limit. Caller
-- should compare CountActiveDevicesForUser against the policy first.
DELETE FROM device_tokens
WHERE id IN (
    SELECT id FROM device_tokens
    WHERE tenant_id = $1 AND user_id = $2
    ORDER BY last_seen_at ASC
    LIMIT 1
);
