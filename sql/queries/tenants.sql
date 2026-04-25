-- name: CreateTenant :one
INSERT INTO tenants (org_code, name, slug, plan, status, owner_user_id, theme, app_config)
VALUES ($1, $2, $3, COALESCE(NULLIF($4::text, ''), 'starter'), 'active', $5, $6, $7)
RETURNING *;

-- name: GetTenantByID :one
SELECT * FROM tenants WHERE id = $1 LIMIT 1;

-- name: GetTenantByOrgCode :one
SELECT * FROM tenants WHERE org_code = upper($1) AND status = 'active' LIMIT 1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1 AND status = 'active' LIMIT 1;

-- name: GetTenantByDomain :one
SELECT * FROM tenants WHERE custom_domain = $1 AND status = 'active' LIMIT 1;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: UpdateTenantBranding :one
UPDATE tenants
SET name = COALESCE(NULLIF($2::text, ''), name),
    logo_url = COALESCE(NULLIF($3::text, ''), logo_url),
    theme = COALESCE($4, theme),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateTenantPlan :one
UPDATE tenants
SET plan = $2,
    status = $3,
    trial_ends_at = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetTenantCustomDomain :one
UPDATE tenants
SET custom_domain = NULLIF($2::text, ''),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetTenantRazorpayAccount :one
-- Used after the tenant completes Linked-Account KYC. Once set, course-buy
-- + subscription-checkout pass a `transfers` block on the Razorpay order so
-- payouts auto-split.
UPDATE tenants
SET razorpay_account_id = NULLIF($2::text, ''),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SuspendTenant :exec
UPDATE tenants SET status = 'suspended', updated_at = now() WHERE id = $1;

-- name: ReactivateTenant :exec
UPDATE tenants SET status = 'active', updated_at = now() WHERE id = $1;

-- name: GetTenantFeatures :one
SELECT features FROM tenant_features WHERE tenant_id = $1 LIMIT 1;

-- name: UpsertTenantFeatures :one
INSERT INTO tenant_features (tenant_id, features)
VALUES ($1, $2)
ON CONFLICT (tenant_id) DO UPDATE
    SET features = EXCLUDED.features,
        updated_at = now()
RETURNING *;
