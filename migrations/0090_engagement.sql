-- 0090_engagement.sql
-- Reviews, forum, course chat, gamification, wishlist, gifts. All under the
-- standard FORCE-RLS + current_tenant_id() policy (the v1 engagement tables
-- used a weaker ENABLE-only inline policy) and all covered by sqlc queries.

CREATE TABLE course_reviews (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id   uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating      smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body        text NOT NULL DEFAULT '',
    is_approved boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (course_id, user_id)
);
CREATE INDEX idx_course_reviews_course ON course_reviews (course_id, created_at DESC);
CREATE INDEX idx_course_reviews_tenant ON course_reviews (tenant_id);
SELECT apply_tenant_table('course_reviews');

CREATE TABLE forum_threads (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id     uuid REFERENCES courses(id) ON DELETE CASCADE,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title         text NOT NULL,
    body          text NOT NULL DEFAULT '',
    is_pinned     boolean NOT NULL DEFAULT false,
    is_locked     boolean NOT NULL DEFAULT false,
    reply_count   integer NOT NULL DEFAULT 0,
    last_reply_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_forum_threads_course ON forum_threads (course_id, last_reply_at DESC NULLS LAST);
CREATE INDEX idx_forum_threads_tenant ON forum_threads (tenant_id);
SELECT apply_tenant_table('forum_threads');

CREATE TABLE forum_posts (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    thread_id               uuid NOT NULL REFERENCES forum_threads(id) ON DELETE CASCADE,
    user_id                 uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body                    text NOT NULL,
    is_instructor_highlight boolean NOT NULL DEFAULT false,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_forum_posts_thread ON forum_posts (thread_id, created_at);
CREATE INDEX idx_forum_posts_tenant ON forum_posts (tenant_id);
SELECT apply_tenant_table('forum_posts');

CREATE TABLE course_chat_messages (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    course_id  uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_course_chat_messages_course ON course_chat_messages (course_id, created_at DESC);
CREATE INDEX idx_course_chat_messages_tenant ON course_chat_messages (tenant_id);
SELECT apply_tenant_rls('course_chat_messages');
SELECT apply_app_grants('course_chat_messages');

-- Platform-level badge catalog.
CREATE TABLE badges (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code        citext NOT NULL UNIQUE,
    name        text NOT NULL,
    description text,
    icon        text,
    points      integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE badges ENABLE ROW LEVEL SECURITY;
ALTER TABLE badges FORCE ROW LEVEL SECURITY;
CREATE POLICY badges_read ON badges FOR SELECT USING (true);
CREATE POLICY badges_super ON badges USING (is_super_admin()) WITH CHECK (is_super_admin());
SELECT apply_updated_at('badges');
SELECT apply_app_grants('badges');

CREATE TABLE badge_grants (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    badge_id   uuid NOT NULL REFERENCES badges(id) ON DELETE CASCADE,
    earned_at  timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, badge_id)
);
CREATE INDEX idx_badge_grants_user ON badge_grants (tenant_id, user_id, earned_at DESC);
SELECT apply_tenant_rls('badge_grants');
SELECT apply_app_grants('badge_grants');

CREATE TABLE learning_streaks (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_active_date date,
    current_streak   integer NOT NULL DEFAULT 0,
    longest_streak   integer NOT NULL DEFAULT 0,
    total_points     integer NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id)
);
CREATE INDEX idx_learning_streaks_user ON learning_streaks (user_id);
SELECT apply_tenant_table('learning_streaks');

CREATE TABLE wishlists (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id  uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, course_id)
);
CREATE INDEX idx_wishlists_user ON wishlists (tenant_id, user_id, created_at DESC);
SELECT apply_tenant_rls('wishlists');
SELECT apply_app_grants('wishlists');

CREATE TABLE course_gifts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    sender_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id        uuid REFERENCES orders(id) ON DELETE SET NULL,
    product_id      uuid REFERENCES products(id) ON DELETE SET NULL,
    recipient_phone citext,
    recipient_email citext,
    redemption_code text NOT NULL UNIQUE,
    redeemed_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    redeemed_at     timestamptz,
    entitlement_id  uuid REFERENCES entitlements(id) ON DELETE SET NULL,
    message         text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_course_gifts_sender ON course_gifts (sender_id, created_at DESC);
CREATE INDEX idx_course_gifts_unredeemed ON course_gifts (recipient_phone) WHERE redeemed_at IS NULL;
SELECT apply_tenant_rls('course_gifts');
SELECT apply_app_grants('course_gifts');
-- Sender or recipient can see the gift.
CREATE POLICY course_gifts_recipient ON course_gifts FOR SELECT
    USING (sender_id = current_app_user() OR redeemed_by = current_app_user());
