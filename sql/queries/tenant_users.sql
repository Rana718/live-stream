-- name: AddTenantUser :one
INSERT INTO tenant_users (tenant_id, user_id, role, status, invited_by)
VALUES ($1, $2, $3, COALESCE(NULLIF($4::text, ''), 'active'), $5)
ON CONFLICT (tenant_id, user_id, role) DO UPDATE
    SET status = EXCLUDED.status,
        joined_at = COALESCE(tenant_users.joined_at, now())
RETURNING *;

-- name: GetTenantUser :one
SELECT * FROM tenant_users
WHERE tenant_id = $1 AND user_id = $2 AND status = 'active'
ORDER BY joined_at ASC
LIMIT 1;

-- name: ListTenantsForUser :many
-- Returns every tenant the user has membership in. Used by the login flow
-- when a user belongs to >1 org so they can pick which to enter.
SELECT t.*, tu.role
FROM tenants t
JOIN tenant_users tu ON tu.tenant_id = t.id
WHERE tu.user_id = $1 AND tu.status = 'active' AND t.status = 'active'
ORDER BY tu.joined_at DESC;

-- name: ListUsersForTenant :many
SELECT u.*, tu.role AS tenant_role
FROM users u
JOIN tenant_users tu ON tu.user_id = u.id
WHERE tu.tenant_id = $1 AND tu.status = 'active'
ORDER BY tu.joined_at DESC
LIMIT $2 OFFSET $3;

-- name: SetTenantUserRole :exec
UPDATE tenant_users
SET role = $3
WHERE tenant_id = $1 AND user_id = $2;

-- name: DeactivateTenantUser :exec
UPDATE tenant_users
SET status = 'inactive'
WHERE tenant_id = $1 AND user_id = $2;
