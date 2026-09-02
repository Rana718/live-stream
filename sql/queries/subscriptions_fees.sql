-- subscriptions_fees.sql — subscription plans + subscriptions, fee plans +
-- fee accounts + installments. Amounts are bigint minor units.

-- ─────────────────────────────────────────────────────── subscription_plans

-- name: CreateSubscriptionPlan :one
INSERT INTO subscription_plans (
    tenant_id, name, slug, description, interval, interval_days, trial_days,
    features, hsn_sac, tax_rate_bps, display_order
)
VALUES ($1, $2, sqlc.arg(slug)::citext, sqlc.narg(description)::text,
        COALESCE(sqlc.narg(interval)::subscription_interval, 'monthly'),
        COALESCE(sqlc.narg(interval_days)::int, 30),
        COALESCE(sqlc.narg(trial_days)::int, 0),
        COALESCE(sqlc.narg(features)::jsonb, '[]'::jsonb),
        sqlc.narg(hsn_sac)::text, COALESCE(sqlc.narg(tax_rate_bps)::int, 0),
        COALESCE(sqlc.narg(display_order)::int, 0))
RETURNING id, tenant_id, name, slug, interval, interval_days, trial_days,
          features, hsn_sac, tax_rate_bps, is_active, display_order;

-- name: GetSubscriptionPlan :one
SELECT id, tenant_id, name, slug, description, interval, interval_days,
       trial_days, features, hsn_sac, tax_rate_bps, is_active
FROM subscription_plans WHERE id = $1 AND deleted_at IS NULL;

-- name: ListActiveSubscriptionPlans :many
SELECT id, name, slug, description, interval, interval_days, trial_days, features
FROM subscription_plans
WHERE tenant_id = $1 AND is_active AND deleted_at IS NULL
ORDER BY display_order, created_at;

-- name: SetSubscriptionPlanActive :exec
UPDATE subscription_plans SET is_active = $2 WHERE id = $1;

-- ─────────────────────────────────────────────────────────── subscriptions

-- name: CreateSubscription :one
INSERT INTO subscriptions (
    tenant_id, user_id, plan_id, status, trial_end, current_period_start,
    current_period_end, origin_order_id, entitlement_id
)
VALUES ($1, $2, $3, $4, sqlc.narg(trial_end)::timestamptz,
        sqlc.narg(current_period_start)::timestamptz,
        sqlc.narg(current_period_end)::timestamptz,
        sqlc.narg(origin_order_id)::uuid, sqlc.narg(entitlement_id)::uuid)
RETURNING id, tenant_id, user_id, plan_id, status, trial_end,
          current_period_start, current_period_end, entitlement_id, created_at;

-- name: GetSubscription :one
SELECT id, tenant_id, user_id, plan_id, status, current_period_start,
       current_period_end, trial_end, cancel_at_period_end, cancelled_at,
       gateway_subscription_id, origin_order_id, latest_order_id, entitlement_id
FROM subscriptions WHERE id = $1;

-- name: GetActiveSubscriptionForUser :one
SELECT id, plan_id, status, current_period_start, current_period_end,
       trial_end, cancel_at_period_end, entitlement_id
FROM subscriptions
WHERE tenant_id = $1 AND user_id = $2
  AND status IN ('trialing','active','past_due')
ORDER BY current_period_end DESC NULLS LAST
LIMIT 1;

-- name: GetSubscriptionByGatewayID :one
SELECT id, tenant_id, user_id, plan_id, status, current_period_end, entitlement_id
FROM subscriptions
WHERE gateway_subscription_id = sqlc.arg(gateway_subscription_id)::text;

-- name: ActivateSubscription :one
UPDATE subscriptions
SET status = 'active',
    current_period_start = $2,
    current_period_end = $3,
    latest_order_id = COALESCE(sqlc.narg(latest_order_id)::uuid, latest_order_id)
WHERE id = $1
RETURNING id, tenant_id, user_id, plan_id, status, current_period_start,
          current_period_end, entitlement_id;

-- name: SetSubscriptionStatus :exec
UPDATE subscriptions SET status = $2,
    cancelled_at = CASE WHEN $2 = 'cancelled'::subscription_status THEN now() ELSE cancelled_at END
WHERE id = $1;

-- name: SetSubscriptionCancelAtPeriodEnd :exec
UPDATE subscriptions SET cancel_at_period_end = $2 WHERE id = $1;

-- name: SetSubscriptionGatewayID :exec
UPDATE subscriptions SET gateway_subscription_id = $2 WHERE id = $1;

-- name: ListSubscriptionsForUser :many
SELECT s.id, s.plan_id, s.status, s.current_period_start, s.current_period_end,
       s.cancelled_at, p.name AS plan_name, p.slug AS plan_slug
FROM subscriptions s JOIN subscription_plans p ON p.id = s.plan_id
WHERE s.tenant_id = $1 AND s.user_id = $2
ORDER BY s.created_at DESC;

-- name: ListExpiringSubscriptions :many
SELECT id, tenant_id, user_id, plan_id, current_period_end, cancel_at_period_end
FROM subscriptions
WHERE status = 'active' AND current_period_end < $1
ORDER BY current_period_end;

-- ─────────────────────────────────────────────────────────────── fee_plans

-- name: CreateFeePlan :one
INSERT INTO fee_plans (
    tenant_id, course_id, batch_id, name, total_minor, installments_count,
    gap_days, late_fee_minor, hsn_sac, tax_rate_bps
)
VALUES ($1, sqlc.narg(course_id)::uuid, sqlc.narg(batch_id)::uuid, $2, $3,
        COALESCE(sqlc.narg(installments_count)::int, 1),
        COALESCE(sqlc.narg(gap_days)::int, 30),
        COALESCE(sqlc.narg(late_fee_minor)::bigint, 0),
        sqlc.narg(hsn_sac)::text, COALESCE(sqlc.narg(tax_rate_bps)::int, 0))
RETURNING id, tenant_id, course_id, batch_id, name, total_minor,
          installments_count, gap_days, late_fee_minor, is_active;

-- name: GetFeePlan :one
SELECT id, tenant_id, course_id, batch_id, name, total_minor, currency,
       installments_count, gap_days, late_fee_minor, hsn_sac, tax_rate_bps, is_active
FROM fee_plans WHERE id = $1 AND deleted_at IS NULL;

-- name: ListFeePlansByCourse :many
SELECT id, name, total_minor, installments_count, gap_days, is_active
FROM fee_plans WHERE tenant_id = $1 AND course_id = $2 AND deleted_at IS NULL
ORDER BY created_at DESC;

-- ─────────────────────────────────────────────────────────── fee_accounts

-- name: CreateFeeAccount :one
INSERT INTO fee_accounts (
    tenant_id, user_id, fee_plan_id, course_id, batch_id, total_minor, due_on
)
VALUES ($1, $2, sqlc.narg(fee_plan_id)::uuid, sqlc.narg(course_id)::uuid,
        sqlc.narg(batch_id)::uuid, $3, sqlc.narg(due_on)::date)
RETURNING id, tenant_id, user_id, fee_plan_id, course_id, batch_id,
          total_minor, paid_minor, waived_minor, status, due_on;

-- name: GetFeeAccount :one
SELECT id, tenant_id, user_id, fee_plan_id, course_id, batch_id, total_minor,
       paid_minor, waived_minor, currency, status, due_on
FROM fee_accounts WHERE id = $1;

-- name: AddFeeAccountPayment :one
UPDATE fee_accounts
SET paid_minor = paid_minor + $2,
    status = CASE
        WHEN paid_minor + $2 + waived_minor >= total_minor THEN 'paid'::fee_account_status
        WHEN paid_minor + $2 > 0 THEN 'partial'::fee_account_status
        ELSE status
    END
WHERE id = $1
RETURNING id, total_minor, paid_minor, waived_minor, status;

-- name: WaiveFeeAccount :exec
UPDATE fee_accounts
SET waived_minor = waived_minor + $2,
    status = CASE WHEN paid_minor + waived_minor + $2 >= total_minor
                  THEN 'waived'::fee_account_status ELSE status END
WHERE id = $1;

-- name: ListFeeAccountsForUser :many
SELECT id, course_id, batch_id, total_minor, paid_minor, waived_minor, status, due_on
FROM fee_accounts WHERE tenant_id = $1 AND user_id = $2
ORDER BY created_at DESC;

-- name: ListPendingFeeAccounts :many
SELECT fa.id, fa.user_id, fa.total_minor, fa.paid_minor, fa.status, fa.due_on,
       u.full_name, u.phone
FROM fee_accounts fa JOIN users u ON u.id = fa.user_id
WHERE fa.tenant_id = $1 AND fa.status IN ('pending','partial','overdue')
ORDER BY fa.due_on NULLS LAST LIMIT $2 OFFSET $3;

-- ─────────────────────────────────────────────────────── fee_installments

-- name: CreateFeeInstallment :one
INSERT INTO fee_installments (tenant_id, fee_account_id, seq, amount_minor, due_on)
VALUES ($1, $2, $3, $4, sqlc.narg(due_on)::date)
RETURNING id, fee_account_id, seq, amount_minor, due_on, status;

-- name: GetFeeInstallment :one
SELECT id, tenant_id, fee_account_id, seq, amount_minor, due_on, status,
       paid_at, order_id
FROM fee_installments WHERE id = $1;

-- name: MarkFeeInstallmentPaid :one
UPDATE fee_installments
SET status = 'paid', paid_at = now(), order_id = sqlc.narg(order_id)::uuid
WHERE id = $1 AND status <> 'paid'
RETURNING id, fee_account_id, seq, amount_minor, status;

-- name: ListFeeInstallments :many
SELECT id, seq, amount_minor, due_on, status, paid_at, order_id
FROM fee_installments WHERE fee_account_id = $1 ORDER BY seq;

-- name: ListOverdueFeeInstallments :many
SELECT fi.id, fi.fee_account_id, fi.seq, fi.amount_minor, fi.due_on,
       fa.user_id, u.full_name, u.phone
FROM fee_installments fi
JOIN fee_accounts fa ON fa.id = fi.fee_account_id
JOIN users u ON u.id = fa.user_id
WHERE fi.tenant_id = $1 AND fi.status = 'pending' AND fi.due_on < current_date
ORDER BY fi.due_on LIMIT $2 OFFSET $3;

-- name: MarkFeeInstallmentsOverdue :exec
UPDATE fee_installments SET status = 'overdue'
WHERE tenant_id = $1 AND status = 'pending' AND due_on < current_date;

-- name: ListFeeInstallmentsForUser :many
SELECT fi.id, fi.fee_account_id, fi.seq, fi.amount_minor, fi.due_on, fi.status, fi.paid_at,
       c.title AS course_title
FROM fee_installments fi
JOIN fee_accounts fa ON fa.id = fi.fee_account_id
LEFT JOIN courses c ON c.id = fa.course_id
WHERE fi.tenant_id = $1 AND fa.user_id = $2
ORDER BY fi.due_on NULLS LAST, fi.seq;
