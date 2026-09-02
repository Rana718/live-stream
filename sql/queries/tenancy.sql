-- tenancy.sql — tenants, tenant_domains, tenant_settings, tenant_users,
-- user_profiles. Org-code / domain lookups run under WithPublicLookup;
-- membership listing at login runs under WithSuperAdmin (no tenant yet).

-- name: GetTenantByID :one
SELECT id, org_code, name, slug, parent_tenant_id, status, plan, logo_url,
       theme, legal_name, gstin, pan, registered_address, place_of_supply,
       billing_email, razorpay_account_id, timezone, locale, trial_ends_at,
       owner_user_id, created_at, updated_at
FROM tenants
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetTenantByOrgCode :one
SELECT id, org_code, name, slug, parent_tenant_id, status, plan, logo_url,
       theme, place_of_supply, timezone, locale, trial_ends_at, created_at
FROM tenants
WHERE org_code = sqlc.arg(org_code)::citext AND deleted_at IS NULL;

-- name: GetTenantByDomain :one
SELECT t.id, t.org_code, t.name, t.slug, t.status, t.plan, t.logo_url, t.theme
FROM tenants t
JOIN tenant_domains d ON d.tenant_id = t.id
WHERE d.domain = sqlc.arg(domain)::citext AND d.verified_at IS NOT NULL
  AND t.deleted_at IS NULL;

-- name: CreateTenant :one
INSERT INTO tenants (org_code, name, slug, plan, status, place_of_supply, timezone, owner_user_id)
VALUES (
    sqlc.arg(org_code)::citext, $1, sqlc.arg(slug)::citext,
    COALESCE(sqlc.narg(plan)::tenant_plan, 'starter'),
    COALESCE(sqlc.narg(status)::tenant_status, 'trial'),
    sqlc.narg(place_of_supply)::text,
    COALESCE(sqlc.narg(timezone)::text, 'Asia/Kolkata'),
    sqlc.narg(owner_user_id)::uuid
)
RETURNING id, org_code, name, slug, status, plan, place_of_supply, created_at;

-- name: UpdateTenantBranding :one
UPDATE tenants
SET name     = COALESCE(sqlc.narg(name)::text, name),
    logo_url = COALESCE(sqlc.narg(logo_url)::text, logo_url),
    theme    = COALESCE(sqlc.narg(theme)::jsonb, theme)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, name, logo_url, theme;

-- name: UpdateTenantBilling :one
UPDATE tenants
SET legal_name         = COALESCE(sqlc.narg(legal_name)::text, legal_name),
    gstin              = COALESCE(sqlc.narg(gstin)::text, gstin),
    pan                = COALESCE(sqlc.narg(pan)::text, pan),
    place_of_supply    = COALESCE(sqlc.narg(place_of_supply)::text, place_of_supply),
    registered_address = COALESCE(sqlc.narg(registered_address)::jsonb, registered_address),
    billing_email      = COALESCE(sqlc.narg(billing_email)::citext, billing_email)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, legal_name, gstin, pan, place_of_supply, registered_address, billing_email;

-- name: SetTenantStatus :exec
UPDATE tenants SET status = $2 WHERE id = $1;

-- name: SetTenantPlan :exec
UPDATE tenants SET plan = $2 WHERE id = $1;

-- name: ListTenants :many
SELECT id, org_code, name, slug, status, plan, created_at
FROM tenants
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- ────────────────────────────────────────────────────────── tenant_domains

-- name: AddTenantDomain :one
INSERT INTO tenant_domains (tenant_id, domain, is_primary)
VALUES ($1, sqlc.arg(domain)::citext, $2)
RETURNING id, tenant_id, domain, is_primary, verified_at, ssl_status;

-- name: VerifyTenantDomain :exec
UPDATE tenant_domains SET verified_at = now(), ssl_status = 'active'
WHERE tenant_id = $1 AND domain = sqlc.arg(domain)::citext;

-- name: ListTenantDomains :many
SELECT id, domain, is_primary, verified_at, ssl_status
FROM tenant_domains WHERE tenant_id = $1 ORDER BY is_primary DESC, created_at;

-- ────────────────────────────────────────────────────────── tenant_settings

-- name: GetTenantSettings :one
SELECT tenant_id, features, theme, payment_config, notification_config, updated_at
FROM tenant_settings WHERE tenant_id = $1;

-- name: UpsertTenantSettings :one
INSERT INTO tenant_settings (tenant_id, features, theme, payment_config, notification_config)
VALUES ($1,
        COALESCE(sqlc.narg(features)::jsonb, '{}'::jsonb),
        COALESCE(sqlc.narg(theme)::jsonb, '{}'::jsonb),
        COALESCE(sqlc.narg(payment_config)::jsonb, '{}'::jsonb),
        COALESCE(sqlc.narg(notification_config)::jsonb, '{}'::jsonb))
ON CONFLICT (tenant_id) DO UPDATE SET
    features            = COALESCE(sqlc.narg(features)::jsonb, tenant_settings.features),
    theme               = COALESCE(sqlc.narg(theme)::jsonb, tenant_settings.theme),
    payment_config      = COALESCE(sqlc.narg(payment_config)::jsonb, tenant_settings.payment_config),
    notification_config = COALESCE(sqlc.narg(notification_config)::jsonb, tenant_settings.notification_config)
RETURNING tenant_id, features, theme, payment_config, notification_config, updated_at;

-- ─────────────────────────────────────────────────────────── tenant_users

-- name: GetTenantUser :one
SELECT id, tenant_id, user_id, role, status, invited_by, joined_at
FROM tenant_users
WHERE tenant_id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- name: ListMembershipsForUser :many
SELECT tu.tenant_id, tu.role, tu.status,
       t.org_code, t.name, t.slug, t.status AS tenant_status
FROM tenant_users tu
JOIN tenants t ON t.id = tu.tenant_id
WHERE tu.user_id = $1 AND tu.deleted_at IS NULL AND t.deleted_at IS NULL
ORDER BY tu.joined_at;

-- name: ListTenantMembers :many
SELECT tu.id, tu.user_id, tu.role, tu.status, tu.joined_at,
       u.full_name, u.email, u.phone
FROM tenant_users tu
JOIN users u ON u.id = tu.user_id
WHERE tu.tenant_id = $1 AND tu.deleted_at IS NULL
  AND (sqlc.narg(role)::tenant_role IS NULL OR tu.role = sqlc.narg(role)::tenant_role)
ORDER BY tu.joined_at DESC
LIMIT $2 OFFSET $3;

-- name: AddTenantUser :one
INSERT INTO tenant_users (tenant_id, user_id, role, status, invited_by)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(status)::membership_status, 'active'),
        sqlc.narg(invited_by)::uuid)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    role = EXCLUDED.role, status = EXCLUDED.status, deleted_at = NULL
RETURNING id, tenant_id, user_id, role, status, joined_at;

-- name: SetTenantUserRole :exec
UPDATE tenant_users SET role = $3 WHERE tenant_id = $1 AND user_id = $2;

-- name: SetTenantUserStatus :exec
UPDATE tenant_users SET status = $3 WHERE tenant_id = $1 AND user_id = $2;

-- name: RemoveTenantUser :exec
UPDATE tenant_users SET deleted_at = now() WHERE tenant_id = $1 AND user_id = $2;

-- ─────────────────────────────────────────────────────────── user_profiles

-- name: GetUserProfile :one
SELECT id, tenant_id, user_id, class_level, board, exam_goal,
       onboarding_completed, guardian_name, guardian_phone, address, meta
FROM user_profiles WHERE tenant_id = $1 AND user_id = $2;

-- name: UpsertUserProfile :one
INSERT INTO user_profiles (
    tenant_id, user_id, class_level, board, exam_goal, onboarding_completed,
    guardian_name, guardian_phone, address, meta
)
VALUES ($1, $2,
        sqlc.narg(class_level)::text, sqlc.narg(board)::text, sqlc.narg(exam_goal)::text,
        COALESCE(sqlc.narg(onboarding_completed)::boolean, false),
        sqlc.narg(guardian_name)::text, sqlc.narg(guardian_phone)::citext,
        COALESCE(sqlc.narg(address)::jsonb, '{}'::jsonb),
        COALESCE(sqlc.narg(meta)::jsonb, '{}'::jsonb))
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    class_level          = COALESCE(sqlc.narg(class_level)::text, user_profiles.class_level),
    board                = COALESCE(sqlc.narg(board)::text, user_profiles.board),
    exam_goal            = COALESCE(sqlc.narg(exam_goal)::text, user_profiles.exam_goal),
    onboarding_completed = COALESCE(sqlc.narg(onboarding_completed)::boolean, user_profiles.onboarding_completed),
    guardian_name        = COALESCE(sqlc.narg(guardian_name)::text, user_profiles.guardian_name),
    guardian_phone       = COALESCE(sqlc.narg(guardian_phone)::citext, user_profiles.guardian_phone),
    address              = COALESCE(sqlc.narg(address)::jsonb, user_profiles.address),
    meta                 = COALESCE(sqlc.narg(meta)::jsonb, user_profiles.meta)
RETURNING id, tenant_id, user_id, class_level, board, exam_goal,
          onboarding_completed, guardian_name, guardian_phone, address, meta;
