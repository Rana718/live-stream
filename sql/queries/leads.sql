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
SET status = $2,
    notes = COALESCE(NULLIF($3::text, ''), notes),
    assigned_to = COALESCE($4, assigned_to)
WHERE id = $1
RETURNING *;

-- name: MarkLeadBookingIntent :one
-- Called from the public marketing page after the prospect picks a Cal.com
-- slot type but before they actually book. We bump status to 'demo' and
-- prepend the slot choice to the notes — the actual booking confirmation
-- comes later via Cal.com webhook (out of scope here).
UPDATE leads
SET status = 'demo',
    notes = CASE
              WHEN notes IS NULL OR notes = '' THEN $2
              ELSE $2 || E'\n---\n' || notes
            END
WHERE id = $1
RETURNING *;
