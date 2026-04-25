-- name: CreateLead :one
-- Public lead capture from the marketing site. No tenant context — leads
-- exist before any tenant is provisioned.
INSERT INTO leads (name, phone, email, institute_name, city, students_count, source, notes)
VALUES ($1, $2, $3, $4, $5, $6, COALESCE(NULLIF($7::text, ''), 'website'), $8)
RETURNING *;

-- name: ListLeads :many
SELECT * FROM leads
WHERE ($1::text = '' OR status = $1)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateLeadStatus :one
UPDATE leads
SET status = $2, notes = COALESCE(NULLIF($3::text, ''), notes)
WHERE id = $1
RETURNING *;
