-- 037_class_schedules.sql
-- Tenant-side class timetable. The instructor sets up a recurring rule
-- ("every Monday + Wednesday at 18:00 IST for 90 min"); a worker walks
-- the schedule once a day and pre-creates `streams` rows for the next
-- N days so students see the upcoming classes in /live without the
-- instructor having to remember to schedule each one.
--
-- We deliberately store recurrence as a small denormalised set instead
-- of an iCal RRULE string — for our use cases (weekday list + time +
-- duration) the RRULE expressiveness is overkill, and a simple
-- `byweekday SMALLINT[]` keeps queries plannable in pure SQL.

CREATE TABLE IF NOT EXISTS class_schedules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    instructor_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id       UUID REFERENCES courses(id) ON DELETE SET NULL,
    batch_id        UUID REFERENCES batches(id) ON DELETE SET NULL,
    title           VARCHAR(200) NOT NULL,
    description     TEXT,
    -- Days of week the class repeats on. ISO 8601: 1=Mon..7=Sun.
    by_weekday      SMALLINT[] NOT NULL DEFAULT '{}',
    -- Local clock the class starts at. Stored as text 'HH:MM' so we don't
    -- get bitten by Postgres TIME's lack of timezone awareness.
    start_local     VARCHAR(5) NOT NULL,
    duration_min    INTEGER NOT NULL DEFAULT 60,
    -- IANA tz name. India defaults to Asia/Kolkata; multi-region tenants
    -- can override per-schedule.
    timezone        VARCHAR(40) NOT NULL DEFAULT 'Asia/Kolkata',
    -- Window the recurrence is active. NULL ends_on = open-ended.
    starts_on       DATE NOT NULL DEFAULT CURRENT_DATE,
    ends_on         DATE,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    -- The materialiser walks every active schedule daily and creates
    -- streams rows for the next 14 days. Stored here so we don't
    -- re-create rows we've already seeded.
    last_materialised_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT now(),
    updated_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_class_schedules_tenant_active
    ON class_schedules(tenant_id, is_active)
    WHERE is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_class_schedules_instructor
    ON class_schedules(instructor_id);

ALTER TABLE class_schedules ENABLE ROW LEVEL SECURITY;
ALTER TABLE class_schedules FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_class_schedules ON class_schedules;
CREATE POLICY tenant_isolation_class_schedules ON class_schedules
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
DROP POLICY IF EXISTS super_admin_class_schedules ON class_schedules;
CREATE POLICY super_admin_class_schedules ON class_schedules
    USING (is_super_admin()) WITH CHECK (is_super_admin());

-- Link table from each materialised stream back to the schedule that
-- generated it. Lets the materialiser de-dupe and lets analytics
-- aggregate "completion % per schedule" cleanly.
ALTER TABLE streams
    ADD COLUMN IF NOT EXISTS schedule_id UUID
        REFERENCES class_schedules(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_streams_schedule
    ON streams(schedule_id) WHERE schedule_id IS NOT NULL;
