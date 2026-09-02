-- 0100_platform_ops.sql
-- Cross-cutting infrastructure tables. audit_logs is partitioned (immutable,
-- append-only). outbox / jobs / webhook_events carry no RLS — they are
-- worker-facing and the worker runs under WithSuperAdmin; app_user still
-- needs DML on them because request handlers write outbox rows inside the
-- business transaction.

-- ─────────────────────────────────────────────────────────────── audit_logs
CREATE TABLE audit_logs (
    id            uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id     uuid,                         -- null = platform-level action
    actor_user_id uuid,
    actor_role    text,
    action        text NOT NULL,
    entity_type   text,
    entity_id     uuid,
    before        jsonb,
    after         jsonb,
    ip            inet,
    user_agent    text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
CREATE TABLE audit_logs_default PARTITION OF audit_logs DEFAULT;
CREATE INDEX idx_audit_logs_tenant_created ON audit_logs (tenant_id, created_at DESC);
CREATE INDEX idx_audit_logs_entity ON audit_logs (entity_type, entity_id);
CREATE INDEX idx_audit_logs_actor ON audit_logs (actor_user_id);

ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_logs_read ON audit_logs FOR SELECT
    USING (tenant_id = current_tenant_id() OR is_super_admin());
CREATE POLICY audit_logs_insert ON audit_logs FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id() OR tenant_id IS NULL OR is_super_admin());
SELECT apply_app_grants('audit_logs');

-- ─────────────────────────────────────────────────────────────────── outbox
CREATE TABLE outbox (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    aggregate_type text NOT NULL,
    aggregate_id   uuid NOT NULL,
    event_type     text NOT NULL,
    tenant_id      uuid,
    payload        jsonb NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    published_at   timestamptz,
    attempts       integer NOT NULL DEFAULT 0,
    last_error     text
);
CREATE INDEX idx_outbox_unpublished ON outbox (id) WHERE published_at IS NULL;
SELECT apply_app_grants('outbox');

-- ─────────────────────────────────────────────────────────────────── jobs
CREATE TABLE jobs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind         text NOT NULL,
    tenant_id    uuid,
    payload      jsonb NOT NULL DEFAULT '{}'::jsonb,
    run_after    timestamptz NOT NULL DEFAULT now(),
    status       text NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','running','done','failed')),
    attempts     integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 5,
    locked_at    timestamptz,
    locked_by    text,
    completed_at timestamptz,
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_jobs_runnable ON jobs (run_after) WHERE status = 'pending';
SELECT apply_updated_at('jobs');
SELECT apply_app_grants('jobs');

-- ─────────────────────────────────────────────────────────────── webhook_events
CREATE TABLE webhook_events (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway       text NOT NULL,
    event_id      text NOT NULL,
    event_type    text NOT NULL,
    payload       jsonb NOT NULL,
    signature_ok  boolean NOT NULL DEFAULT false,
    received_at   timestamptz NOT NULL DEFAULT now(),
    processed_at  timestamptz,
    process_error text,
    UNIQUE (gateway, event_id)
);
CREATE INDEX idx_webhook_events_unprocessed ON webhook_events (received_at) WHERE processed_at IS NULL;
SELECT apply_app_grants('webhook_events');
