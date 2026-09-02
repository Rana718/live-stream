-- commerce.sql — products, prices, coupons, orders, order_items, payments,
-- refunds, entitlements, enrollments. All amounts are bigint minor units.
-- Every statement is tenant-scoped by RLS; the WHERE clauses are defence in
-- depth and for clarity in logs.

-- ─────────────────────────────────────────────────────────────── products

-- name: GetProduct :one
SELECT id, tenant_id, kind, course_id, bundle_id, plan_id, fee_plan_id,
       hsn_sac, tax_rate_bps, is_active
FROM products
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetProductForCourse :one
SELECT id, tenant_id, kind, course_id, hsn_sac, tax_rate_bps, is_active
FROM products WHERE course_id = $1 AND deleted_at IS NULL;

-- name: GetProductForBundle :one
SELECT id, tenant_id, kind, bundle_id, hsn_sac, tax_rate_bps, is_active
FROM products WHERE bundle_id = $1 AND deleted_at IS NULL;

-- name: GetProductForPlan :one
SELECT id, tenant_id, kind, plan_id, hsn_sac, tax_rate_bps, is_active
FROM products WHERE plan_id = $1 AND deleted_at IS NULL;

-- name: GetProductForFeePlan :one
SELECT id, tenant_id, kind, fee_plan_id, hsn_sac, tax_rate_bps, is_active
FROM products WHERE fee_plan_id = $1 AND deleted_at IS NULL;

-- name: CreateProduct :one
INSERT INTO products (tenant_id, kind, course_id, bundle_id, plan_id, fee_plan_id, hsn_sac, tax_rate_bps)
VALUES ($1, $2,
        sqlc.narg(course_id)::uuid, sqlc.narg(bundle_id)::uuid,
        sqlc.narg(plan_id)::uuid, sqlc.narg(fee_plan_id)::uuid,
        sqlc.narg(hsn_sac)::text, COALESCE(sqlc.narg(tax_rate_bps)::int, 0))
RETURNING id, tenant_id, kind, course_id, bundle_id, plan_id, fee_plan_id, hsn_sac, tax_rate_bps, is_active;

-- name: SetProductActive :exec
UPDATE products SET is_active = $2 WHERE id = $1;

-- name: ListBundleItemProducts :many
SELECT p.id, p.kind, p.course_id, p.bundle_id, p.plan_id, p.fee_plan_id
FROM bundle_items bi
JOIN products p ON p.id = bi.item_product_id
WHERE bi.bundle_product_id = $1
ORDER BY bi.position;

-- name: AddBundleItem :exec
INSERT INTO bundle_items (tenant_id, bundle_product_id, item_product_id, position)
VALUES ($1, $2, $3, $4)
ON CONFLICT (bundle_product_id, item_product_id) DO UPDATE SET position = EXCLUDED.position;

-- ────────────────────────────────────────────────────────────────── prices

-- name: GetActivePrice :one
SELECT id, tenant_id, product_id, amount_minor, compare_at_minor, currency
FROM prices
WHERE product_id = $1 AND is_active AND deleted_at IS NULL
  AND valid_from <= now() AND (valid_to IS NULL OR valid_to > now());

-- name: UpsertActivePrice :one
INSERT INTO prices (tenant_id, product_id, amount_minor, compare_at_minor, currency)
VALUES ($1, $2, $3, sqlc.narg(compare_at_minor)::bigint, 'INR')
RETURNING id, tenant_id, product_id, amount_minor, compare_at_minor, currency;

-- name: DeactivatePricesForProduct :exec
UPDATE prices SET is_active = false WHERE product_id = $1 AND is_active;

-- ───────────────────────────────────────────────────────────────── coupons

-- name: GetCouponByCode :one
SELECT id, tenant_id, code, type, percent_bps, value_minor, max_discount_minor,
       min_order_minor, applies_to, starts_at, ends_at, usage_limit,
       per_user_limit, used_count, is_active
FROM coupons
WHERE tenant_id = $1 AND code = sqlc.arg(code)::citext;

-- name: CountUserCouponRedemptions :one
SELECT count(*) FROM coupon_redemptions WHERE coupon_id = $1 AND user_id = $2;

-- name: IncrementCouponUsage :exec
UPDATE coupons SET used_count = used_count + 1 WHERE id = $1;

-- name: RecordCouponRedemption :one
INSERT INTO coupon_redemptions (tenant_id, coupon_id, user_id, order_id, amount_off_minor)
VALUES ($1, $2, $3, sqlc.narg(order_id)::uuid, $4)
RETURNING id, coupon_id, user_id, order_id, amount_off_minor, created_at;

-- name: CouponAllowsProduct :one
SELECT
    NOT EXISTS (SELECT 1 FROM coupon_products cp WHERE cp.coupon_id = sqlc.arg(coupon_id)::uuid)
    OR EXISTS (
        SELECT 1 FROM coupon_products cp
        WHERE cp.coupon_id = sqlc.arg(coupon_id)::uuid AND cp.product_id = sqlc.arg(product_id)::uuid
    );

-- ────────────────────────────────────────────────────────────────── orders

-- name: CreateOrder :one
INSERT INTO orders (
    tenant_id, user_id, code, status, subtotal_minor, discount_minor,
    tax_minor, total_minor, currency, coupon_id, gateway, gateway_order_id,
    place_of_supply, notes, refund_deadline_at, placed_at
)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(status)::order_status, 'pending'),
        $4, $5, $6, $7, 'INR',
        sqlc.narg(coupon_id)::uuid, sqlc.narg(gateway)::text,
        sqlc.narg(gateway_order_id)::text, sqlc.narg(place_of_supply)::text,
        COALESCE(sqlc.narg(notes)::jsonb, '{}'::jsonb),
        sqlc.narg(refund_deadline_at)::timestamptz, now())
RETURNING id, tenant_id, user_id, code, status, subtotal_minor, discount_minor,
          tax_minor, total_minor, currency, coupon_id, gateway, gateway_order_id,
          place_of_supply, refund_deadline_at, placed_at, paid_at, created_at;

-- name: GetOrderByID :one
SELECT id, tenant_id, user_id, code, status, subtotal_minor, discount_minor,
       tax_minor, total_minor, currency, coupon_id, gateway, gateway_order_id,
       place_of_supply, refund_deadline_at, placed_at, paid_at, cancelled_at, created_at
FROM orders WHERE id = $1;

-- name: GetOrderByGatewayOrderID :one
SELECT id, tenant_id, user_id, code, status, subtotal_minor, discount_minor,
       tax_minor, total_minor, currency, coupon_id, gateway, gateway_order_id,
       place_of_supply, paid_at, created_at
FROM orders WHERE gateway_order_id = sqlc.arg(gateway_order_id)::text;

-- name: GetOrderByGatewayOrderIDForUpdate :one
SELECT id, tenant_id, user_id, code, status, total_minor, currency,
       gateway_order_id, place_of_supply, paid_at, created_at
FROM orders WHERE gateway_order_id = sqlc.arg(gateway_order_id)::text
FOR UPDATE;

-- name: MarkOrderPaid :one
UPDATE orders SET status = 'paid', paid_at = now()
WHERE id = $1 AND status <> 'paid'
RETURNING id, tenant_id, user_id, status, total_minor, place_of_supply, paid_at;

-- name: SetOrderStatus :exec
UPDATE orders SET status = $2 WHERE id = $1;

-- name: NextOrderSequence :one
SELECT count(*) + 1 FROM orders WHERE tenant_id = $1;

-- name: ListOrdersForUser :many
SELECT id, code, status, total_minor, currency, gateway, paid_at, created_at
FROM orders WHERE tenant_id = $1 AND user_id = $2
ORDER BY created_at DESC LIMIT $3 OFFSET $4;

-- name: ListTenantOrders :many
SELECT o.id, o.code, o.status, o.total_minor, o.currency, o.paid_at, o.created_at,
       u.full_name, u.email, u.phone
FROM orders o JOIN users u ON u.id = o.user_id
WHERE o.tenant_id = $1
  AND (sqlc.narg(status)::order_status IS NULL OR o.status = sqlc.narg(status)::order_status)
ORDER BY o.created_at DESC LIMIT $2 OFFSET $3;

-- ─────────────────────────────────────────────────────────────── order_items

-- name: CreateOrderItem :one
INSERT INTO order_items (
    tenant_id, order_id, product_id, product_kind, title, hsn_sac, unit_minor,
    qty, line_subtotal_minor, discount_minor, taxable_minor, cgst_minor,
    sgst_minor, igst_minor, total_minor, grants_entitlement, entitlement_days
)
VALUES ($1, $2, $3, $4, $5, sqlc.narg(hsn_sac)::text, $6, $7, $8,
        $9, $10, $11, $12, $13, $14,
        COALESCE(sqlc.narg(grants_entitlement)::boolean, true),
        sqlc.narg(entitlement_days)::int)
RETURNING id, order_id, product_id, product_kind, title, unit_minor, qty,
          taxable_minor, cgst_minor, sgst_minor, igst_minor, total_minor,
          grants_entitlement, entitlement_days;

-- name: ListOrderItems :many
SELECT id, order_id, product_id, product_kind, title, hsn_sac, unit_minor, qty,
       line_subtotal_minor, discount_minor, taxable_minor, cgst_minor,
       sgst_minor, igst_minor, total_minor, grants_entitlement, entitlement_days
FROM order_items WHERE order_id = $1 ORDER BY created_at;

-- ─────────────────────────────────────────────────────────────── payments

-- name: CreatePayment :one
INSERT INTO payments (
    tenant_id, order_id, user_id, gateway, gateway_order_id, amount_minor, currency, status
)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(gateway)::text, 'razorpay'),
        sqlc.narg(gateway_order_id)::text, $4, 'INR',
        COALESCE(sqlc.narg(status)::payment_status, 'created'))
RETURNING id, tenant_id, order_id, user_id, gateway, gateway_order_id,
          gateway_payment_id, method, status, amount_minor, currency, created_at;

-- name: GetPaymentByID :one
SELECT id, tenant_id, order_id, user_id, gateway, gateway_order_id,
       gateway_payment_id, method, status, amount_minor, currency,
       gateway_fee_minor, captured_at, created_at
FROM payments WHERE id = $1;

-- name: MarkPaymentCaptured :one
UPDATE payments
SET status = 'captured', gateway_payment_id = $2, signature = sqlc.narg(signature)::text,
    method = sqlc.narg(method)::payment_method, captured_at = now(), raw = COALESCE(sqlc.narg(raw)::jsonb, raw)
WHERE id = $1
RETURNING id, tenant_id, order_id, user_id, status, amount_minor, captured_at;

-- name: MarkPaymentFailed :exec
UPDATE payments SET status = 'failed', failure_reason = $2, failed_at = now()
WHERE id = $1;

-- name: ListPaymentsForOrder :many
SELECT id, gateway, gateway_payment_id, method, status, amount_minor,
       gateway_fee_minor, captured_at, created_at
FROM payments WHERE order_id = $1 ORDER BY created_at;

-- name: CreatePaymentSplit :exec
INSERT INTO payment_splits (tenant_id, payment_id, linked_account_id, amount_minor, on_hold, gateway_transfer_id)
VALUES ($1, $2, $3, $4, COALESCE(sqlc.narg(on_hold)::boolean, false), sqlc.narg(gateway_transfer_id)::text);

-- ─────────────────────────────────────────────────────────────── refunds

-- name: CreateRefund :one
INSERT INTO refunds (tenant_id, payment_id, order_item_id, amount_minor, reason, status, initiated_by)
VALUES ($1, $2, sqlc.narg(order_item_id)::uuid, $3,
        COALESCE(sqlc.narg(reason)::refund_reason, 'requested_by_customer'),
        'pending', sqlc.narg(initiated_by)::uuid)
RETURNING id, tenant_id, payment_id, amount_minor, currency, reason, status, created_at;

-- name: MarkRefundProcessed :one
UPDATE refunds SET status = 'processed', gateway_refund_id = $2, processed_at = now()
WHERE id = $1
RETURNING id, payment_id, amount_minor, status, gateway_refund_id, processed_at;

-- name: MarkRefundFailed :exec
UPDATE refunds SET status = 'failed', notes = $2 WHERE id = $1;

-- name: GetRefundByGatewayID :one
SELECT id, tenant_id, payment_id, amount_minor, status
FROM refunds WHERE gateway_refund_id = sqlc.arg(gateway_refund_id)::text;

-- name: SumRefundedForPayment :one
SELECT COALESCE(sum(amount_minor), 0)::bigint
FROM refunds WHERE payment_id = $1 AND status IN ('pending','processing','processed');

-- ─────────────────────────────────────────────────────────── entitlements

-- name: GrantEntitlement :one
INSERT INTO entitlements (
    tenant_id, user_id, product_id, product_kind, source, order_item_id, source_ref, expires_at, created_by
)
VALUES ($1, $2, $3, $4, $5, sqlc.narg(order_item_id)::uuid, sqlc.narg(source_ref)::uuid,
        sqlc.narg(expires_at)::timestamptz, sqlc.narg(created_by)::uuid)
RETURNING id, tenant_id, user_id, product_id, product_kind, source, order_item_id,
          granted_at, expires_at, revoked_at;

-- name: RevokeEntitlement :exec
UPDATE entitlements SET revoked_at = now(), revoke_reason = $2
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeEntitlementsForOrderItem :exec
UPDATE entitlements SET revoked_at = now(), revoke_reason = $2
WHERE order_item_id = $1 AND revoked_at IS NULL;

-- name: CheckEntitlement :one
SELECT EXISTS (
    SELECT 1 FROM entitlements
    WHERE tenant_id = $1 AND user_id = $2 AND product_id = $3
      AND revoked_at IS NULL
      AND (expires_at IS NULL OR expires_at > now())
);

-- name: ExtendEntitlement :exec
UPDATE entitlements SET expires_at = $2 WHERE id = $1;

-- name: ListUserEntitlements :many
SELECT e.id, e.product_id, e.product_kind, e.source, e.granted_at, e.expires_at,
       p.course_id, p.bundle_id, p.plan_id
FROM entitlements e JOIN products p ON p.id = e.product_id
WHERE e.tenant_id = $1 AND e.user_id = $2 AND e.revoked_at IS NULL
  AND (e.expires_at IS NULL OR e.expires_at > now())
ORDER BY e.granted_at DESC;

-- ─────────────────────────────────────────────────────────────── enrollments

-- name: UpsertEnrollment :one
INSERT INTO enrollments (tenant_id, user_id, course_id, batch_id, entitlement_id, status, started_at)
VALUES ($1, $2, $3, sqlc.narg(batch_id)::uuid, sqlc.narg(entitlement_id)::uuid, 'active', now())
ON CONFLICT (tenant_id, user_id, course_id) DO UPDATE SET
    status = 'active',
    batch_id = COALESCE(EXCLUDED.batch_id, enrollments.batch_id),
    entitlement_id = COALESCE(EXCLUDED.entitlement_id, enrollments.entitlement_id)
RETURNING id, tenant_id, user_id, course_id, batch_id, entitlement_id, status, progress_bps, started_at;

-- name: GetEnrollment :one
SELECT id, tenant_id, user_id, course_id, batch_id, entitlement_id, status,
       progress_bps, started_at, completed_at, expires_at
FROM enrollments WHERE tenant_id = $1 AND user_id = $2 AND course_id = $3;

-- name: SetEnrollmentProgress :exec
UPDATE enrollments
SET progress_bps = $4,
    status = CASE WHEN $4 >= 10000 THEN 'completed'::enrollment_status ELSE status END,
    completed_at = CASE WHEN $4 >= 10000 AND completed_at IS NULL THEN now() ELSE completed_at END
WHERE tenant_id = $1 AND user_id = $2 AND course_id = $3;

-- name: CancelEnrollment :exec
UPDATE enrollments SET status = 'cancelled'
WHERE tenant_id = $1 AND user_id = $2 AND course_id = $3;

-- name: ListEnrollmentsForUser :many
SELECT e.id, e.course_id, e.batch_id, e.status, e.progress_bps, e.started_at, e.completed_at,
       c.title, c.slug, c.thumbnail_url
FROM enrollments e JOIN courses c ON c.id = e.course_id
WHERE e.tenant_id = $1 AND e.user_id = $2 AND e.status <> 'cancelled'
ORDER BY e.started_at DESC NULLS LAST;

-- name: ListCourseRoster :many
SELECT e.id, e.user_id, e.batch_id, e.status, e.progress_bps, e.started_at,
       u.full_name, u.email, u.phone
FROM enrollments e JOIN users u ON u.id = e.user_id
WHERE e.tenant_id = $1 AND e.course_id = $2
ORDER BY e.started_at DESC NULLS LAST LIMIT $3 OFFSET $4;

-- ─────────────────────────────────────────────────── coupon admin CRUD

-- name: CreateCoupon :one
INSERT INTO coupons (tenant_id, code, type, percent_bps, value_minor, max_discount_minor,
                     min_order_minor, applies_to, starts_at, ends_at, usage_limit, per_user_limit)
VALUES ($1, sqlc.arg(code)::citext, $2, sqlc.narg(percent_bps)::int, sqlc.narg(value_minor)::bigint,
        sqlc.narg(max_discount_minor)::bigint, COALESCE(sqlc.narg(min_order_minor)::bigint, 0),
        COALESCE(sqlc.narg(applies_to)::coupon_scope, 'all'),
        COALESCE(sqlc.narg(starts_at)::timestamptz, now()), sqlc.narg(ends_at)::timestamptz,
        sqlc.narg(usage_limit)::int, COALESCE(sqlc.narg(per_user_limit)::int, 1))
RETURNING id, tenant_id, code, type, percent_bps, value_minor, max_discount_minor,
          min_order_minor, applies_to, starts_at, ends_at, usage_limit, per_user_limit,
          used_count, is_active;

-- name: ListCoupons :many
SELECT id, code, type, percent_bps, value_minor, max_discount_minor, min_order_minor,
       applies_to, starts_at, ends_at, usage_limit, per_user_limit, used_count, is_active
FROM coupons WHERE tenant_id = $1
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: SetCouponActive :exec
UPDATE coupons SET is_active = $2 WHERE id = $1 AND tenant_id = $3;

-- name: DeleteCoupon :exec
DELETE FROM coupons WHERE id = $1 AND tenant_id = $2;

-- name: AttachCouponToProduct :exec
INSERT INTO coupon_products (tenant_id, coupon_id, product_id) VALUES ($1, $2, $3)
ON CONFLICT (coupon_id, product_id) DO NOTHING;

-- ─────────────────────────────────────────────────── course_bundles

-- name: CreateCourseBundle :one
INSERT INTO course_bundles (tenant_id, title, description, cover_url, display_order)
VALUES ($1, $2, sqlc.narg(description)::text, sqlc.narg(cover_url)::text,
        COALESCE(sqlc.narg(display_order)::int, 0))
RETURNING id, tenant_id, title, description, cover_url, is_active, display_order;

-- name: GetCourseBundle :one
SELECT id, tenant_id, title, description, cover_url, is_active, display_order
FROM course_bundles WHERE id = $1;

-- name: ListCourseBundles :many
SELECT id, title, description, cover_url, is_active, display_order
FROM course_bundles WHERE tenant_id = $1 AND is_active
ORDER BY display_order, created_at DESC;

-- name: AdminListCourseBundles :many
SELECT id, title, description, cover_url, is_active, display_order
FROM course_bundles WHERE tenant_id = $1
ORDER BY display_order, created_at DESC;

-- name: SetCourseBundleActive :exec
UPDATE course_bundles SET is_active = $2 WHERE id = $1 AND tenant_id = $3;

-- name: DeleteCourseBundle :exec
DELETE FROM course_bundles WHERE id = $1 AND tenant_id = $2;

-- name: ListBundleCourses :many
SELECT c.id, c.title, c.slug, c.thumbnail_url
FROM bundle_items bi
JOIN products ip ON ip.id = bi.item_product_id AND ip.course_id IS NOT NULL
JOIN courses c ON c.id = ip.course_id
WHERE bi.bundle_product_id = $1
ORDER BY bi.position;

-- name: AdminListPayments :many
SELECT p.id, p.order_id, p.user_id, p.gateway, p.gateway_payment_id, p.gateway_order_id,
       p.method, p.status, p.amount_minor, p.currency, p.captured_at, p.created_at,
       o.code AS order_code,
       u.full_name, u.email, u.phone,
       (SELECT COALESCE(sum(r.amount_minor),0)::bigint FROM refunds r
          WHERE r.payment_id = p.id AND r.status IN ('pending','processing','processed')) AS refunded_minor
FROM payments p
JOIN orders o ON o.id = p.order_id
JOIN users u ON u.id = p.user_id
WHERE p.tenant_id = $1
  AND (sqlc.narg(status)::payment_status IS NULL OR p.status = sqlc.narg(status)::payment_status)
ORDER BY p.created_at DESC
LIMIT $2 OFFSET $3;

-- name: PlatformListPayments :many
-- Cross-tenant payment list for the super-admin console. Runs under
-- WithSuperAdmin (RLS bypass); never mount on a tenant route.
SELECT p.id, p.tenant_id, p.order_id, p.user_id, p.gateway, p.gateway_payment_id,
       p.method, p.status, p.amount_minor, p.currency, p.captured_at, p.created_at,
       o.code AS order_code,
       u.full_name, u.email, u.phone,
       t.org_code, t.name AS tenant_name
FROM payments p
JOIN orders o ON o.id = p.order_id
JOIN users u ON u.id = p.user_id
JOIN tenants t ON t.id = p.tenant_id
WHERE (sqlc.narg(status)::payment_status IS NULL OR p.status = sqlc.narg(status)::payment_status)
  AND (sqlc.narg(tenant_id)::uuid IS NULL OR p.tenant_id = sqlc.narg(tenant_id)::uuid)
ORDER BY p.created_at DESC
LIMIT $1 OFFSET $2;

-- name: GetPaymentByIDForTenant :one
SELECT p.id, p.tenant_id, p.order_id, p.user_id, p.gateway_payment_id, p.status, p.amount_minor
FROM payments p WHERE p.id = $1 AND p.tenant_id = $2;

-- name: FirstGrantsEntitlementOrderItem :one
SELECT id, product_id, product_kind FROM order_items
WHERE order_id = $1 AND grants_entitlement ORDER BY created_at LIMIT 1;
