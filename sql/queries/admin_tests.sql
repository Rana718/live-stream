-- name: SetTestPublished :one
-- Admin moderation: publish or take down an individual test. Used by
-- /admin/tests/:id/publish PATCH.
UPDATE tests
SET is_published = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;
