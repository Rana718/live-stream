-- name: CreateSubscriptionPlan :one
INSERT INTO subscription_plans (name, slug, description, price, currency, duration_days, features, display_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetPlanByID :one
SELECT * FROM subscription_plans WHERE id = $1 LIMIT 1;

-- name: GetPlanBySlug :one
SELECT * FROM subscription_plans WHERE slug = $1 LIMIT 1;

-- name: ListActivePlans :many
SELECT * FROM subscription_plans WHERE is_active = TRUE ORDER BY display_order ASC, price ASC;

-- name: UpdatePlan :one
UPDATE subscription_plans
SET name = $2, description = $3, price = $4, duration_days = $5, features = $6,
    is_active = $7, display_order = $8, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeletePlan :exec
DELETE FROM subscription_plans WHERE id = $1;

-- name: CreateUserSubscription :one
INSERT INTO user_subscriptions (user_id, plan_id, status, starts_at, ends_at, auto_renew)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSubscriptionByID :one
SELECT * FROM user_subscriptions WHERE id = $1 LIMIT 1;

-- name: GetActiveSubscription :one
SELECT * FROM user_subscriptions
WHERE user_id = $1 AND status = 'active' AND ends_at > CURRENT_TIMESTAMP
ORDER BY ends_at DESC
LIMIT 1;

-- name: ListUserSubscriptions :many
SELECT us.*, p.name AS plan_name, p.slug AS plan_slug
FROM user_subscriptions us
JOIN subscription_plans p ON p.id = us.plan_id
WHERE us.user_id = $1
ORDER BY us.created_at DESC;

-- name: ActivateSubscription :one
UPDATE user_subscriptions
SET status = 'active', starts_at = CURRENT_TIMESTAMP,
    ends_at = CURRENT_TIMESTAMP + make_interval(days => $2),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: CancelSubscription :exec
UPDATE user_subscriptions
SET status = 'cancelled', cancelled_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: GetUserSubByProviderID :one
-- Razorpay webhooks reference the subscription by their internal id
-- (sub_XXX). We store it on user_subscriptions when the subscription is
-- created, so the webhook handler can resolve the local row without a
-- per-event mapping table.
SELECT * FROM user_subscriptions
WHERE razorpay_subscription_id = $1
LIMIT 1;

-- name: SetUserSubStatusByProviderID :one
-- Patches just the status. We don't touch ends_at on cancellation so
-- the user keeps access until their paid period ends; cancellation only
-- prevents the next renewal.
UPDATE user_subscriptions
SET status = $2,
    updated_at = CURRENT_TIMESTAMP,
    cancelled_at = CASE WHEN $2 IN ('cancelled', 'completed', 'halted')
                        THEN COALESCE(cancelled_at, CURRENT_TIMESTAMP)
                        ELSE cancelled_at END
WHERE razorpay_subscription_id = $1
RETURNING *;

-- name: CreatePayment :one
INSERT INTO payments (user_id, subscription_id, amount, currency, provider, provider_order_id, status, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetPaymentByID :one
SELECT * FROM payments WHERE id = $1 LIMIT 1;

-- name: GetPaymentByProviderOrderID :one
SELECT * FROM payments WHERE provider_order_id = $1 LIMIT 1;

-- name: ListUserPayments :many
SELECT * FROM payments WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: UpdatePaymentStatus :one
UPDATE payments
SET status = $2, provider_payment_id = $3, provider_signature = $4, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;
