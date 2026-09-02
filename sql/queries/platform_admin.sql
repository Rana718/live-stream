-- platform_admin.sql — super_admin (platform-staff) control plane.
-- Every query here runs under WithSuperAdmin (is_super_admin() = true), so
-- tenant RLS is bypassed and results span every tenant on the platform.

-- name: PlatformListTenants :many
SELECT t.id, t.org_code, t.name, t.slug, t.status, t.plan,
       t.billing_email, t.razorpay_account_id, t.trial_ends_at,
       t.created_at,
       (SELECT count(*) FROM tenant_users tu
          WHERE tu.tenant_id = t.id AND tu.deleted_at IS NULL) AS member_count
FROM tenants t
WHERE t.deleted_at IS NULL
  AND (sqlc.narg(status)::tenant_status IS NULL OR t.status = sqlc.narg(status)::tenant_status)
ORDER BY t.created_at DESC
LIMIT $1 OFFSET $2;

-- name: PlatformTenantStats :one
SELECT
    count(*)                                              AS total_tenants,
    count(*) FILTER (WHERE status = 'active')             AS active_tenants,
    count(*) FILTER (WHERE status = 'trial')              AS trial_tenants,
    count(*) FILTER (WHERE status = 'suspended')          AS suspended_tenants,
    (SELECT count(*) FROM users WHERE deleted_at IS NULL) AS total_users,
    (SELECT count(*) FROM tenant_users WHERE deleted_at IS NULL) AS total_memberships
FROM tenants
WHERE deleted_at IS NULL;

-- name: PlatformLeadStats :one
SELECT
    count(*)                                       AS total_leads,
    count(*) FILTER (WHERE status = 'new')         AS new_leads,
    count(*) FILTER (WHERE status = 'contacted')   AS contacted_leads,
    count(*) FILTER (WHERE status = 'qualified')   AS qualified_leads,
    count(*) FILTER (WHERE status = 'converted')   AS converted_leads,
    count(*) FILTER (WHERE status = 'lost')        AS lost_leads
FROM leads;

-- name: PlatformRecentSignups :many
SELECT u.id, u.full_name, u.email, u.phone, u.created_at,
       tu.role, t.org_code, t.name AS tenant_name
FROM users u
JOIN tenant_users tu ON tu.user_id = u.id AND tu.deleted_at IS NULL
JOIN tenants t ON t.id = tu.tenant_id
WHERE u.deleted_at IS NULL
ORDER BY u.created_at DESC
LIMIT $1;

-- name: PlatformListUsers :many
SELECT u.id, u.full_name, u.email, u.phone, u.status, u.created_at, u.last_login_at,
       tu.role, tu.status AS membership_status,
       t.id AS tenant_id, t.org_code, t.name AS tenant_name
FROM users u
JOIN tenant_users tu ON tu.user_id = u.id AND tu.deleted_at IS NULL
JOIN tenants t ON t.id = tu.tenant_id AND t.deleted_at IS NULL
WHERE u.deleted_at IS NULL
  AND (sqlc.narg(tenant_id)::uuid IS NULL OR t.id = sqlc.narg(tenant_id)::uuid)
  AND (sqlc.narg(role)::tenant_role IS NULL OR tu.role = sqlc.narg(role)::tenant_role)
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.full_name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR u.email::text ILIKE '%' || sqlc.narg(q)::text || '%'
    OR u.phone::text ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY u.created_at DESC
LIMIT $1 OFFSET $2;

-- name: PlatformCountUsers :one
SELECT count(*)
FROM users u
JOIN tenant_users tu ON tu.user_id = u.id AND tu.deleted_at IS NULL
JOIN tenants t ON t.id = tu.tenant_id AND t.deleted_at IS NULL
WHERE u.deleted_at IS NULL
  AND (sqlc.narg(tenant_id)::uuid IS NULL OR t.id = sqlc.narg(tenant_id)::uuid)
  AND (sqlc.narg(role)::tenant_role IS NULL OR tu.role = sqlc.narg(role)::tenant_role)
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.full_name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR u.email::text ILIKE '%' || sqlc.narg(q)::text || '%'
    OR u.phone::text ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- name: SetTenantRazorpayAccount :one
UPDATE tenants
SET razorpay_account_id = nullif(sqlc.arg(razorpay_account_id)::text, '')
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, org_code, name, slug, status, plan, razorpay_account_id;

-- name: SetTenantPrimaryDomain :exec
INSERT INTO tenant_domains (tenant_id, domain, is_primary, verified_at, ssl_status)
VALUES ($1, sqlc.arg(domain)::citext, true, now(), 'active')
ON CONFLICT (domain) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id, is_primary = true, verified_at = now();

-- name: DeleteTenantDomain :exec
DELETE FROM tenant_domains WHERE tenant_id = $1 AND domain = sqlc.arg(domain)::citext;

-- name: PlatformUpdateTenantPlan :one
UPDATE tenants
SET plan          = sqlc.arg(plan)::tenant_plan,
    status        = sqlc.arg(status)::tenant_status,
    trial_ends_at = sqlc.narg(trial_ends_at)::timestamptz
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, org_code, name, slug, status, plan, trial_ends_at;
