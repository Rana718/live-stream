-- name: WriteAuditLog :one
INSERT INTO audit_logs (tenant_id, actor_id, actor_role, action, resource_type, resource_id, ip, user_agent, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListAuditLogsForActor :many
SELECT * FROM audit_logs
WHERE actor_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAuditLogsForTenant :many
SELECT * FROM audit_logs
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: VerifyUserEmail :one
UPDATE users
SET email_verified = TRUE, email_verified_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: RecordUserLogin :exec
UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = $1;
