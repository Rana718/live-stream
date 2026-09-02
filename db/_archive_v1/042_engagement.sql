-- Engagement features: reviews, forum, gamification, wishlist, gifts,
-- lecture notes, course chat, WhatsApp 2-way, affiliate payouts.
-- All queries against these tables go through raw pgx in their handlers
-- (sqlc regen blocked by file ownership) so RETURNING * works without
-- a generated row struct.

-- ───────────────────────────────────────────────────────── reviews
CREATE TABLE IF NOT EXISTS course_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body TEXT NOT NULL DEFAULT '',
    is_approved BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (course_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_reviews_course ON course_reviews (course_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_reviews_tenant ON course_reviews (tenant_id);
ALTER TABLE course_reviews ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_course_reviews ON course_reviews;
CREATE POLICY rls_course_reviews ON course_reviews USING (
    tenant_id = current_setting('app.tenant_id', TRUE)::uuid
    OR current_setting('app.is_super_admin', TRUE) = 'true'
);

-- ───────────────────────────────────────────────────────── forum
CREATE TABLE IF NOT EXISTS forum_threads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    is_pinned BOOLEAN NOT NULL DEFAULT FALSE,
    is_locked BOOLEAN NOT NULL DEFAULT FALSE,
    reply_count INT NOT NULL DEFAULT 0,
    last_reply_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_forum_threads_course ON forum_threads (course_id, last_reply_at DESC NULLS LAST);
ALTER TABLE forum_threads ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_forum_threads ON forum_threads;
CREATE POLICY rls_forum_threads ON forum_threads USING (
    tenant_id = current_setting('app.tenant_id', TRUE)::uuid
    OR current_setting('app.is_super_admin', TRUE) = 'true'
);

CREATE TABLE IF NOT EXISTS forum_posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    thread_id UUID NOT NULL REFERENCES forum_threads(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    is_instructor_highlight BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_forum_posts_thread ON forum_posts (thread_id, created_at);
ALTER TABLE forum_posts ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_forum_posts ON forum_posts;
CREATE POLICY rls_forum_posts ON forum_posts USING (
    tenant_id = current_setting('app.tenant_id', TRUE)::uuid
    OR current_setting('app.is_super_admin', TRUE) = 'true'
);

-- ───────────────────────────────────────────────────────── gamification
-- Badges are platform-wide (not tenant-scoped) so the catalog is shared.
CREATE TABLE IF NOT EXISTS badges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT UNIQUE NOT NULL,         -- "first_test", "30_day_streak", ...
    name TEXT NOT NULL,
    description TEXT,
    icon TEXT,                         -- lucide icon name
    points INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO badges (code, name, description, icon, points) VALUES
    ('first_test', 'First Test', 'Completed your first practice test', 'rocket', 10),
    ('seven_day_streak', '7-day streak', 'Studied 7 days in a row', 'flame', 50),
    ('thirty_day_streak', '30-day streak', 'Studied 30 days in a row', 'trophy', 250),
    ('test_topper', 'Topper', 'Scored 90%+ on any test', 'medal', 100),
    ('first_purchase', 'Enrolled', 'Purchased your first course', 'badge-check', 25),
    ('helpful_answerer', 'Helpful', 'Posted an instructor-highlighted forum reply', 'lightbulb', 75)
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS badge_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    badge_id UUID NOT NULL REFERENCES badges(id) ON DELETE CASCADE,
    earned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, badge_id)
);
CREATE INDEX IF NOT EXISTS idx_badge_grants_user ON badge_grants (user_id, earned_at DESC);

CREATE TABLE IF NOT EXISTS user_streaks (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_active_date DATE,
    current_streak INT NOT NULL DEFAULT 0,
    longest_streak INT NOT NULL DEFAULT 0,
    total_points INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ───────────────────────────────────────────────────────── wishlist
CREATE TABLE IF NOT EXISTS wishlists (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, course_id)
);
CREATE INDEX IF NOT EXISTS idx_wishlists_user ON wishlists (user_id, created_at DESC);

-- ───────────────────────────────────────────────────────── gifts
CREATE TABLE IF NOT EXISTS course_gifts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_phone TEXT,
    recipient_email TEXT,
    course_id UUID REFERENCES courses(id) ON DELETE SET NULL,
    bundle_id UUID,
    amount_paise BIGINT NOT NULL DEFAULT 0,
    redemption_code TEXT UNIQUE NOT NULL,
    redeemed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    redeemed_at TIMESTAMPTZ,
    razorpay_payment_id TEXT,
    message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_course_gifts_sender ON course_gifts (sender_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_course_gifts_phone ON course_gifts (recipient_phone) WHERE redeemed_at IS NULL;
ALTER TABLE course_gifts ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_course_gifts ON course_gifts;
CREATE POLICY rls_course_gifts ON course_gifts USING (
    tenant_id = current_setting('app.tenant_id', TRUE)::uuid
    OR current_setting('app.is_super_admin', TRUE) = 'true'
);

-- ───────────────────────────────────────────────────────── lecture notes
-- Timestamped student notes / highlights on lecture videos.
CREATE TABLE IF NOT EXISTS lecture_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lecture_id UUID NOT NULL REFERENCES lectures(id) ON DELETE CASCADE,
    timestamp_seconds INT NOT NULL DEFAULT 0,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_notes_user_lecture ON lecture_notes (user_id, lecture_id, timestamp_seconds);

-- ───────────────────────────────────────────────────────── course chat
-- Course-scoped study room chat. Polled by the client on a timer; we
-- don't add a new WebSocket route since coaching cohorts rarely need
-- live keystroke-level chat outside of the live class itself.
CREATE TABLE IF NOT EXISTS course_chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_course_chat_course ON course_chat_messages (course_id, created_at DESC);
ALTER TABLE course_chat_messages ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_course_chat ON course_chat_messages;
CREATE POLICY rls_course_chat ON course_chat_messages USING (
    tenant_id = current_setting('app.tenant_id', TRUE)::uuid
    OR current_setting('app.is_super_admin', TRUE) = 'true'
);

-- ───────────────────────────────────────────────────────── WhatsApp 2-way
-- Inbound + outbound transactional message log keyed by phone. Reused
-- by both the broadcast channel and the new 2-way inbox.
CREATE TABLE IF NOT EXISTS wa_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    direction TEXT NOT NULL CHECK (direction IN ('in', 'out')),
    phone TEXT NOT NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    body TEXT NOT NULL,
    provider_id TEXT,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_wa_phone ON wa_messages (phone, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_wa_unread ON wa_messages (tenant_id, is_read) WHERE direction = 'in' AND is_read = FALSE;

-- ───────────────────────────────────────────────────────── affiliate
-- Extends the existing referrals module with payout tracking.
CREATE TABLE IF NOT EXISTS affiliate_payouts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount_paise BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending', -- pending | paid | rejected
    method TEXT,                            -- upi | bank | wallet
    note TEXT,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_payouts_user ON affiliate_payouts (user_id, requested_at DESC);

-- ───────────────────────────────────────────────────────── practice tests
-- Tag the existing `test_attempts` row instead of a new table — keeps
-- analytics consistent and lets the same submission flow score both
-- modes. Practice attempts are excluded from leaderboards and reports.
ALTER TABLE test_attempts ADD COLUMN IF NOT EXISTS is_practice BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_test_attempts_real ON test_attempts (test_id) WHERE is_practice = FALSE;
