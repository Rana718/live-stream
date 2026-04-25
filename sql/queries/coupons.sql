-- name: CreateCoupon :one
INSERT INTO coupons (
    tenant_id, code, discount_type, discount_value, max_discount,
    scope, min_amount, starts_at, ends_at, usage_limit
) VALUES ($1, upper($2), $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetCouponByCode :one
SELECT * FROM coupons
WHERE tenant_id = $1 AND code = upper($2) AND is_active = TRUE
LIMIT 1;

-- name: ListCoupons :many
SELECT * FROM coupons
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: SetCouponActive :exec
UPDATE coupons SET is_active = $2 WHERE id = $1;

-- name: IncrementCouponUsage :exec
UPDATE coupons SET used_count = used_count + 1 WHERE id = $1;

-- name: DeleteCoupon :exec
DELETE FROM coupons WHERE id = $1;

-- name: AttachCouponToCourse :exec
INSERT INTO coupon_courses (coupon_id, course_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListCouponCourses :many
SELECT course_id FROM coupon_courses WHERE coupon_id = $1;

-- name: RecordCouponRedemption :one
INSERT INTO coupon_redemptions (tenant_id, coupon_id, user_id, payment_id, amount_off)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CountCouponRedemptionsByUser :one
SELECT count(*) FROM coupon_redemptions
WHERE coupon_id = $1 AND user_id = $2;
