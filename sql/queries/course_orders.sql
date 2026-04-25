-- name: CreateCourseOrder :one
INSERT INTO payments (
    tenant_id, user_id, course_id, amount, currency,
    provider, provider_order_id, status, metadata
) VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5::text, ''), 'INR'),
          'razorpay', $6, 'created', $7)
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
