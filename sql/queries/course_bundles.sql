-- name: ListCourseBundles :many
-- Active bundles for the student store. RLS handles tenant scoping.
SELECT
    b.id,
    b.title,
    b.description,
    b.price_paise,
    b.cover_url,
    b.display_order,
    b.created_at,
    -- Aggregate course IDs and sum of individual prices (in paise) so the
    -- API can compute the "save ₹X" sticker without a second round trip.
    -- Coalesce to handle a bundle with no courses (admin in progress).
    COALESCE(array_agg(ci.course_id) FILTER (WHERE ci.course_id IS NOT NULL), ARRAY[]::uuid[]) AS course_ids,
    COALESCE(SUM((c.price * 100)::int) FILTER (WHERE c.id IS NOT NULL), 0)::int AS member_price_paise
FROM course_bundles b
LEFT JOIN course_bundle_items ci ON ci.bundle_id = b.id
LEFT JOIN courses c              ON c.id = ci.course_id AND c.is_published = TRUE
WHERE b.is_active = TRUE
GROUP BY b.id
ORDER BY b.display_order, b.created_at DESC;

-- name: GetCourseBundleByID :one
-- Single bundle, used by the buy endpoint for price + tenant validation.
SELECT * FROM course_bundles WHERE id = $1;

-- name: ListCourseBundleItems :many
-- Course IDs in a bundle. Buy/verify uses this to fan out enrollments.
SELECT course_id FROM course_bundle_items WHERE bundle_id = $1;

-- name: CreateCourseBundle :one
-- Admin-only. Used by /admin/bundles or the seed script.
INSERT INTO course_bundles (tenant_id, title, description, price_paise, cover_url, display_order, is_active)
VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, TRUE))
RETURNING *;

-- name: AddCourseToBundle :exec
-- Idempotent — re-running with the same (bundle, course) does nothing.
INSERT INTO course_bundle_items (bundle_id, course_id)
VALUES ($1, $2)
ON CONFLICT (bundle_id, course_id) DO NOTHING;

-- name: RemoveCourseFromBundle :exec
DELETE FROM course_bundle_items WHERE bundle_id = $1 AND course_id = $2;

-- name: SetBundleActive :exec
UPDATE course_bundles SET is_active = $2, updated_at = now() WHERE id = $1;
