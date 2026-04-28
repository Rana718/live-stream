-- name: CreateCourseOrder :one
INSERT INTO payments (
    tenant_id, user_id, course_id, amount, currency,
    provider, provider_order_id, status, metadata
) VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5::text, ''), 'INR'),
          'razorpay', $6, 'created', $7)
RETURNING *;

-- name: CreateBundleOrder :one
-- Same as CreateCourseOrder but with course_id NULL — bundles fan out
-- to multiple courses on verify, so the link from the payment row to
-- the bundle lives in metadata.bundle_id rather than a single FK.
INSERT INTO payments (
    tenant_id, user_id, course_id, amount, currency,
    provider, provider_order_id, status, metadata
) VALUES ($1, $2, NULL, $3, COALESCE(NULLIF($4::text, ''), 'INR'),
          'razorpay', $5, 'created', $6)
RETURNING *;

-- name: GetCourseOrderByProviderOrderID :one
SELECT * FROM payments WHERE provider_order_id = $1 LIMIT 1;

-- name: MarkCourseOrderPaid :one
UPDATE payments
SET status = 'paid',
    provider_payment_id = $2,
    provider_signature = $3,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: HasUserBoughtCourse :one
SELECT EXISTS (
    SELECT 1 FROM payments
    WHERE user_id = $1 AND course_id = $2 AND status = 'paid'
);

-- name: GetPaymentByIDForTenant :one
-- Tenant-scoped lookup. RLS would also block cross-tenant reads but the
-- explicit WHERE is defence in depth (and clearer in logs).
SELECT * FROM payments
WHERE id = $1 AND tenant_id = $2
LIMIT 1;

-- name: MarkPaymentRefunded :one
-- Records the refund alongside the original payment row. We don't
-- introduce a separate refunds table — a row's lifecycle
-- (created → paid → refunded) is captured by status transitions plus
-- a `refund` block in metadata holding the razorpay refund id, amount
-- and reason.
UPDATE payments
SET status = 'refunded',
    metadata = jsonb_set(
        COALESCE(metadata, '{}'::jsonb),
        '{refund}',
        jsonb_build_object(
            'razorpay_refund_id', $2::text,
            'amount_paise',       $3::bigint,
            'reason',             $4::text,
            'refunded_at',        to_jsonb(CURRENT_TIMESTAMP)
        )
    ),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;
