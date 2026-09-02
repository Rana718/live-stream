-- admin.sql — tenant_admin control plane (RLS-scoped to the caller's tenant
-- via explicit tenant_id predicates + the FORCE RLS policy).

-- name: AdminListTenantMembers :many
SELECT u.id, u.full_name, u.email, u.phone, u.status, u.created_at, u.last_login_at,
       tu.role, tu.status AS membership_status
FROM tenant_users tu
JOIN users u ON u.id = tu.user_id AND u.deleted_at IS NULL
WHERE tu.tenant_id = $1 AND tu.deleted_at IS NULL
  AND (sqlc.narg(role)::tenant_role IS NULL OR tu.role = sqlc.narg(role)::tenant_role)
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.full_name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR u.email::text ILIKE '%' || sqlc.narg(q)::text || '%'
    OR u.phone::text ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY u.created_at DESC
LIMIT $2 OFFSET $3;

-- name: AdminGetTenantMember :one
SELECT u.id, u.full_name, u.email, u.phone, u.status, u.created_at,
       tu.role, tu.status AS membership_status
FROM tenant_users tu
JOIN users u ON u.id = tu.user_id
WHERE tu.tenant_id = $1 AND tu.user_id = $2 AND tu.deleted_at IS NULL;

-- name: AdminBatchAttendance :many
SELECT a.batch_id,
       count(*)::bigint                                          AS total,
       count(*) FILTER (WHERE a.status IN ('present','late'))::bigint AS attended,
       (100.0 * count(*) FILTER (WHERE a.status IN ('present','late'))
        / NULLIF(count(*), 0))::float                            AS attendance_percent
FROM attendance a
WHERE a.tenant_id = $1 AND a.batch_id IS NOT NULL
GROUP BY a.batch_id;
