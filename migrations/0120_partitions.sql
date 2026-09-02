-- 0120_partitions.sql
-- Seed the current + next two monthly partitions for every partitioned
-- table, and queue an ongoing partition-creation job for the worker. The
-- DEFAULT partitions already exist (created with each parent) so inserts
-- never fail before this runs.

DO $$
DECLARE
    parent  text;
    m       integer;
    base    date := date_trunc('month', now())::date;
BEGIN
    FOREACH parent IN ARRAY ARRAY['session_messages','test_responses','audit_logs'] LOOP
        FOR m IN 0..2 LOOP
            PERFORM create_month_partition(
                format('public.%I', parent)::regclass,
                (base + make_interval(months => m))::date
            );
        END LOOP;
    END LOOP;
END $$;

INSERT INTO jobs (kind, payload)
VALUES ('create_partitions',
        jsonb_build_object(
            'tables', jsonb_build_array('session_messages','test_responses','audit_logs'),
            'ahead_months', 3));
