-- name: FanOutToAllTenantStudents :exec
-- Tenant-wide notification fan-out. Inserts one row per student in
-- the current tenant. RLS scopes the SELECT side to the caller's
-- tenant — the admin invoking this gets exactly their tenant's
-- students, no cross-tenant spill.
INSERT INTO notifications (user_id, type, title, body, resource_id)
SELECT u.id, $1::text, $2::text, $3::text, $4
FROM users u
WHERE u.role = 'student' AND u.is_active = TRUE;
