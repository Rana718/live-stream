-- name: AdminListPayments :many
-- List payments for the current tenant. RLS scopes by tenant_id; we
-- filter by status optionally for the refunds UI's tab split.
SELECT
    p.id,
    p.user_id,
    u.full_name,
    u.phone_number,
    p.course_id,
    c.title       AS course_title,
    p.amount,
    p.currency,
    p.status,
    p.provider_payment_id,
    p.created_at,
    p.metadata
FROM payments p
LEFT JOIN users u   ON u.id = p.user_id
LEFT JOIN courses c ON c.id = p.course_id
ORDER BY p.created_at DESC
LIMIT $1 OFFSET $2;
