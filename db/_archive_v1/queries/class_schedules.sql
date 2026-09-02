-- name: CreateClassSchedule :one
INSERT INTO class_schedules (
    tenant_id, instructor_id, course_id, batch_id, title, description,
    by_weekday, start_local, duration_min, timezone, starts_on, ends_on
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: ListClassSchedules :many
-- Per-tenant list, used by the admin dashboard. `active_filter` accepts
-- a nullable bool via sqlc.narg — pass nil for "all", true for active,
-- false for paused. We avoid `WHERE … IS NULL OR …` against a typed
-- param because sqlc resolves $1 to a non-nullable type.
SELECT * FROM class_schedules
WHERE (sqlc.narg('active_filter')::bool IS NULL
       OR is_active = sqlc.narg('active_filter')::bool)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: GetClassSchedule :one
SELECT * FROM class_schedules WHERE id = $1 LIMIT 1;

-- name: SetClassScheduleActive :exec
UPDATE class_schedules SET is_active = $2, updated_at = now() WHERE id = $1;

-- name: DeleteClassSchedule :exec
DELETE FROM class_schedules WHERE id = $1;

-- name: ListActiveSchedulesNeedingMaterialisation :many
-- Fed to the daily materialiser worker. We pull schedules that are
-- active, in-window, and either have never been materialised or whose
-- last materialisation was >12 hours ago. The 12-hour gap leaves room
-- for retries without thundering herd.
SELECT * FROM class_schedules
WHERE is_active = TRUE
  AND starts_on <= CURRENT_DATE
  AND (ends_on IS NULL OR ends_on >= CURRENT_DATE)
  AND (last_materialised_at IS NULL OR last_materialised_at < now() - interval '12 hours')
ORDER BY tenant_id, id
LIMIT $1;

-- name: TouchScheduleMaterialised :exec
UPDATE class_schedules
SET last_materialised_at = now()
WHERE id = $1;

-- name: StreamExistsForSchedule :one
-- Idempotency check used by the materialiser. We don't want two streams
-- for the same (schedule, scheduled_at).
SELECT EXISTS (
    SELECT 1 FROM streams
    WHERE schedule_id = $1 AND scheduled_at = $2
);

-- name: CreateScheduledStream :one
-- Used by the materialiser to drop a `streams` row for one occurrence
-- of a schedule. The stream_key is freshly random; status starts as
-- 'scheduled' and only flips to 'live' when nginx-rtmp's auth callback
-- fires StartStreamByKey.
INSERT INTO streams (
    tenant_id, instructor_id, title, description, stream_key,
    status, scheduled_at, schedule_id
) VALUES ($1, $2, $3, $4, encode(gen_random_bytes(16), 'hex'),
          'scheduled', $5, $6)
RETURNING *;
