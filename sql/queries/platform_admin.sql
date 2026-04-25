-- Platform-level admin queries. These cross tenant boundaries so they're
-- only safe to call inside the SuperAdminContext middleware which sets
-- app.is_super_admin and bypasses RLS.

-- name: PlatformListTenants :many
SELECT t.*,
    (SELECT count(*) FROM tenant_users tu WHERE tu.tenant_id = t.id AND tu.status = 'active') AS member_count,
    COALESCE(ps.plan, t.plan) AS billing_plan,
    ps.status AS billing_status,
    ps.current_period_end
FROM tenants t
LEFT JOIN LATERAL (
    SELECT plan, status, current_period_end
    FROM platform_subscriptions
    WHERE tenant_id = t.id
    ORDER BY created_at DESC
    LIMIT 1
) ps ON TRUE
WHERE ($1::text = '' OR t.status = $1)
ORDER BY t.created_at DESC
LIMIT $2 OFFSET $3;

-- name: PlatformTenantStats :one
SELECT
    (SELECT count(*) FROM tenants WHERE status = 'active') AS active_tenants,
    (SELECT count(*) FROM tenants WHERE status = 'trial') AS trial_tenants,
    (SELECT count(*) FROM tenants WHERE status = 'suspended') AS suspended_tenants,
    (SELECT count(*) FROM tenants) AS total_tenants,
    (SELECT count(*) FROM users) AS total_users,
    (SELECT count(*) FROM courses) AS total_courses,
    (SELECT count(*) FROM streams WHERE status = 'live') AS live_streams_now,
    (SELECT COALESCE(sum(amount), 0) FROM platform_subscriptions WHERE status = 'active') AS active_mrr;

-- name: PlatformLeadStats :one
SELECT
    count(*) FILTER (WHERE status = 'new')       AS new_leads,
    count(*) FILTER (WHERE status = 'contacted') AS contacted_leads,
    count(*) FILTER (WHERE status = 'demo')      AS demo_leads,
    count(*) FILTER (WHERE status = 'won')       AS won_leads,
    count(*) FILTER (WHERE status = 'lost')      AS lost_leads,
    count(*)                                     AS total_leads
FROM leads;

-- name: PlatformRecentSignups :many
SELECT u.id, u.full_name, u.phone_number, u.email, u.role, u.created_at,
    t.name AS tenant_name, t.org_code
FROM users u
JOIN tenants t ON u.tenant_id = t.id
ORDER BY u.created_at DESC
LIMIT $1;

-- name: PlatformAuditLogs :many
SELECT al.*, t.name AS tenant_name, t.org_code, u.full_name AS actor_name
FROM audit_logs al
LEFT JOIN tenants t ON t.id = al.tenant_id
LEFT JOIN users u ON u.id = al.actor_id
ORDER BY al.created_at DESC
LIMIT $1 OFFSET $2;

-- ============ App builds (white-label per-tenant Play Store apps) ============

-- name: CreateAppBuild :one
INSERT INTO app_builds (tenant_id, status, platform, package_id, version_name)
VALUES ($1, 'queued', $2, $3, $4)
RETURNING *;

-- name: ListAppBuilds :many
SELECT b.*, t.name AS tenant_name, t.org_code
FROM app_builds b
JOIN tenants t ON t.id = b.tenant_id
WHERE ($1::text = '' OR b.status = $1)
ORDER BY b.created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAppBuildsForTenant :many
SELECT * FROM app_builds WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2;

-- name: SetAppBuildStatus :one
UPDATE app_builds
SET status = $2,
    build_url = COALESCE(NULLIF($3::text, ''), build_url),
    play_url = COALESCE(NULLIF($4::text, ''), play_url),
    error_log = COALESCE(NULLIF($5::text, ''), error_log),
    completed_at = CASE WHEN $2 IN ('published', 'failed') THEN now() ELSE completed_at END
WHERE id = $1
RETURNING *;

-- ============ Platform subscriptions (we bill the tenants) ============

-- name: UpsertPlatformSubscription :one
INSERT INTO platform_subscriptions (tenant_id, plan, status, current_period_end, razorpay_subscription_id, amount, trial_ends_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListPlatformSubscriptions :many
SELECT ps.*, t.name AS tenant_name, t.org_code
FROM platform_subscriptions ps
JOIN tenants t ON t.id = ps.tenant_id
ORDER BY ps.created_at DESC
LIMIT $1 OFFSET $2;

-- name: GetActivePlatformSubscriptionForTenant :one
SELECT * FROM platform_subscriptions
WHERE tenant_id = $1 AND status = 'active'
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateLeadStatus :one
UPDATE leads
SET status = $2,
    notes = COALESCE(NULLIF($3::text, ''), notes),
    assigned_to = COALESCE($4, assigned_to)
WHERE id = $1
RETURNING *;
