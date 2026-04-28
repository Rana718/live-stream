-- name: AdminListBundles :many
-- Admin listing — same shape as the public ListCourseBundles but
-- without the is_active filter so admins see paused entries too.
SELECT
    b.id,
    b.title,
    b.description,
    b.price_paise,
    b.cover_url,
    b.display_order,
    b.is_active,
    b.created_at,
    COALESCE(array_agg(ci.course_id) FILTER (WHERE ci.course_id IS NOT NULL), ARRAY[]::uuid[]) AS course_ids,
    COALESCE(SUM((c.price * 100)::int) FILTER (WHERE c.id IS NOT NULL), 0)::int AS member_price_paise
FROM course_bundles b
LEFT JOIN course_bundle_items ci ON ci.bundle_id = b.id
LEFT JOIN courses c              ON c.id = ci.course_id
GROUP BY b.id
ORDER BY b.display_order, b.created_at DESC;

-- name: DeleteCourseBundle :exec
DELETE FROM course_bundles WHERE id = $1;

-- name: ReplaceBundleItems :exec
-- Atomically replace the membership of a bundle. Used when admin edits
-- the picker. We delete-then-insert inside a transaction in the service.
DELETE FROM course_bundle_items WHERE bundle_id = $1;
