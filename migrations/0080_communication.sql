-- 0080_communication.sql
-- In-app notifications + per-channel delivery tracking, announcements,
-- device push tokens, and 2-way WhatsApp threads. notifications is plain at
-- launch (partition once it crosses ~5M rows); a scheduleworker job prunes
-- rows older than 90 days.

CREATE TABLE notifications (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_key text NOT NULL,
    title       text NOT NULL,
    body        text,
    data        jsonb NOT NULL DEFAULT '{}'::jsonb,
    entity_type text,
    entity_id   uuid,
    read_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_tenant_user ON notifications (tenant_id, user_id, created_at DESC);
CREATE INDEX idx_notifications_unread ON notifications (user_id) WHERE read_at IS NULL;
SELECT apply_tenant_rls('notifications');
SELECT apply_app_grants('notifications');

CREATE TABLE notification_deliveries (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    notification_id    uuid NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel            notification_channel NOT NULL,
    status             delivery_status NOT NULL DEFAULT 'queued',
    provider_message_id text,
    error              text,
    sent_at            timestamptz,
    delivered_at       timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notification_deliveries_notification ON notification_deliveries (notification_id);
CREATE INDEX idx_notification_deliveries_tenant ON notification_deliveries (tenant_id);
SELECT apply_tenant_table('notification_deliveries');

CREATE TABLE notification_preferences (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel    notification_channel NOT NULL,
    category   text NOT NULL,
    enabled    boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, channel, category)
);
CREATE INDEX idx_notification_preferences_user ON notification_preferences (user_id);
SELECT apply_tenant_table('notification_preferences');

CREATE TABLE announcements (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    course_id    uuid REFERENCES courses(id) ON DELETE CASCADE,
    batch_id     uuid REFERENCES batches(id) ON DELETE CASCADE,
    title        text NOT NULL,
    body         text NOT NULL,
    audience     jsonb NOT NULL DEFAULT '{}'::jsonb,
    priority     text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high')),
    scheduled_at timestamptz,
    published_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_announcements_tenant_published ON announcements (tenant_id, published_at DESC);
CREATE INDEX idx_announcements_course ON announcements (course_id);
CREATE INDEX idx_announcements_batch ON announcements (batch_id);
SELECT apply_tenant_table('announcements');

CREATE TABLE device_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token        text NOT NULL UNIQUE,
    platform     text NOT NULL CHECK (platform IN ('android','ios','web')),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_device_tokens_user ON device_tokens (tenant_id, user_id) WHERE revoked_at IS NULL;
SELECT apply_tenant_rls('device_tokens');
SELECT apply_app_grants('device_tokens');

CREATE TABLE messaging_threads (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id         uuid REFERENCES users(id) ON DELETE SET NULL,
    channel         notification_channel NOT NULL,
    phone           citext NOT NULL,
    last_message_at timestamptz,
    unread_count    integer NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, channel, phone)
);
CREATE INDEX idx_messaging_threads_tenant_unread ON messaging_threads (tenant_id) WHERE unread_count > 0;
SELECT apply_tenant_table('messaging_threads');

CREATE TABLE messaging_messages (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    thread_id   uuid NOT NULL REFERENCES messaging_threads(id) ON DELETE CASCADE,
    direction   message_direction NOT NULL,
    body        text NOT NULL,
    provider_id text,
    status      delivery_status NOT NULL DEFAULT 'queued',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_messaging_messages_thread ON messaging_messages (thread_id, created_at DESC);
CREATE INDEX idx_messaging_messages_tenant ON messaging_messages (tenant_id);
SELECT apply_tenant_rls('messaging_messages');
SELECT apply_app_grants('messaging_messages');
