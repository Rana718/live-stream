-- 041_cms_content.sql
-- Marketing-side content: blog posts, FAQs, free-form CMS pages (terms,
-- privacy, etc). NOT tenant-scoped — the marketing site (vidyawarrior.com)
-- is platform-level and the same content is served to every prospect.
--
-- We use a single `cms_pages` table for arbitrary slugged pages instead
-- of one table per page type so super-admins can add new pages (e.g.
-- /refund-policy, /cookies) without DDL.
--
-- Body is stored as TipTap JSON. The marketing site renders it on the
-- server with `@tiptap/html`-style serialiser; the admin UI edits it in
-- TipTap directly. Plain HTML would be a footgun (XSS / sanitisation),
-- and Markdown loses TipTap's structured nodes (callouts, embeds) that
-- a senior content editor wants.

CREATE TABLE IF NOT EXISTS blog_posts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         VARCHAR(200) UNIQUE NOT NULL,
    title        VARCHAR(300) NOT NULL,
    excerpt      TEXT,
    -- TipTap JSON document. Validated in the API layer; raw HTML never
    -- touches storage.
    body_json    JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Pre-rendered HTML cache so the public read endpoint doesn't have
    -- to reach for TipTap on every request. Refreshed by the writer.
    body_html    TEXT NOT NULL DEFAULT '',
    cover_url    TEXT,
    author_name  VARCHAR(200),
    tags         TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    -- Posts can be drafted and revised before publish. Public reads
    -- filter on `published_at IS NOT NULL AND published_at <= now()`.
    published_at TIMESTAMPTZ,
    minutes_read INTEGER NOT NULL DEFAULT 3,
    seo_title    VARCHAR(300),
    seo_desc     VARCHAR(500),
    created_by   UUID,
    created_at   TIMESTAMPTZ DEFAULT now(),
    updated_at   TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_blog_posts_published ON blog_posts(published_at DESC NULLS LAST);
CREATE INDEX IF NOT EXISTS idx_blog_posts_slug      ON blog_posts(slug);
CREATE INDEX IF NOT EXISTS idx_blog_posts_tags      ON blog_posts USING gin(tags);

-- FAQs render as the homepage's accordion + a /faq page. We support
-- categories so the admin can group ("Pricing", "Onboarding", ...).
-- `display_order` controls in-category sort; lower first.
CREATE TABLE IF NOT EXISTS faqs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category      VARCHAR(80) NOT NULL DEFAULT 'general',
    question      VARCHAR(500) NOT NULL,
    answer_html   TEXT NOT NULL,
    -- Whether to surface this on the homepage (a curated short list)
    -- vs. only on /faq (the full set).
    show_on_home  BOOLEAN NOT NULL DEFAULT FALSE,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    display_order INTEGER NOT NULL DEFAULT 100,
    created_at    TIMESTAMPTZ DEFAULT now(),
    updated_at    TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_faqs_category_order
    ON faqs(category, display_order)
    WHERE is_active = TRUE;

-- Generic CMS pages keyed by slug. /terms, /privacy, /refund-policy,
-- /cookies, anything an admin spins up.
CREATE TABLE IF NOT EXISTS cms_pages (
    slug         VARCHAR(100) PRIMARY KEY,
    title        VARCHAR(300) NOT NULL,
    body_json    JSONB NOT NULL DEFAULT '{}'::jsonb,
    body_html    TEXT NOT NULL DEFAULT '',
    seo_title    VARCHAR(300),
    seo_desc     VARCHAR(500),
    is_published BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at   TIMESTAMPTZ DEFAULT now()
);

-- Seed a few starter posts so the marketing site has something to render
-- before the team writes anything. Idempotent via ON CONFLICT.
INSERT INTO blog_posts (slug, title, excerpt, body_html, body_json, author_name, tags, published_at, minutes_read)
VALUES
    ('self-hosted-vs-classplus',
     'Why we self-host nginx-rtmp instead of paying 100ms',
     'How a ₹2,000/mo VPS replaces ₹50,000/mo of WebRTC infra for the broadcast use-case.',
     '<p>Most coaching live classes are <strong>broadcast</strong>, not conferences. One instructor, hundreds of viewers, no cross-talk. WebRTC ($0.004/min/viewer on 100ms) optimises for the wrong thing — sub-100ms latency we don''t need — and you pay for it linearly.</p><p>nginx-rtmp + HLS is built for exactly this shape: ingest one RTMP stream, fan out as HLS to a CDN. We run it on a ₹2,000/mo Hetzner box and serve thousands of viewers per recording. The end-to-end latency is ~6 seconds, which matters for a kabaddi match but not for a Hindi grammar class.</p><h2>The numbers</h2><p>For 500 students watching a 90-minute class three times a week:</p><ul><li>WebRTC: ~₹54,000/month</li><li>HLS via our setup: ~₹600/month CDN egress</li></ul>',
     '{"type":"doc","content":[]}'::jsonb,
     'Vidya Warrior team',
     ARRAY['infra','live-streaming'],
     now() - interval '14 days',
     4),
    ('razorpay-route-for-coaching-payouts',
     'Razorpay Route in 30 lines of Go',
     'Setting up automatic platform commission + tenant payouts on every course sale.',
     '<p>The whole point of Route is that you split a single payment across multiple linked accounts at capture time. No reconciliation cron, no manual settlement spreadsheet.</p><p>Our setup: tenant has a Linked Account, we have the platform account. Each course-sale Razorpay order ships with a <code>transfers</code> array — the tenant share goes straight to their bank, our cut stays.</p>',
     '{"type":"doc","content":[]}'::jsonb,
     'Vidya Warrior team',
     ARRAY['payments','razorpay'],
     now() - interval '7 days',
     6),
    ('from-classplus-to-vidya-warrior',
     'Migrating 500 students off Classplus in a weekend',
     'What we did, what broke, and the 4 things we''d do differently next time.',
     '<p>The move was driven by economics — Classplus charged ₹3 lakh/year + 30% commission, we charge ₹35,000/year + 2%. Math wasn''t the hard part.</p><p>The hard part was Saturday night. Here''s the runbook we shipped on Sunday.</p>',
     '{"type":"doc","content":[]}'::jsonb,
     'Rajan Sir',
     ARRAY['migration','classplus'],
     now() - interval '3 days',
     5)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO faqs (category, question, answer_html, show_on_home, display_order) VALUES
    ('onboarding', 'How long does it take to actually launch?',
     '<p>If you sign up Monday morning, your tenant + admin is provisioned within an hour. Branded Play Store build takes 24–48 hours. iOS adds ~3 days for App Store review.</p>',
     TRUE, 10),
    ('cost', 'What does it cost to run per month at scale?',
     '<p>Our infra cost is roughly <strong>₹500/month per active tenant</strong> — Postgres, Redis, MinIO, nginx-rtmp on a single VPS. CDN egress (HLS) scales linearly with viewers; expect ₹2–3 per GB.</p>',
     TRUE, 20),
    ('migration', 'Can I migrate from Classplus / Vedantu / Physics Wallah?',
     '<p>Yes. Export your student list as CSV, drop it into our admin import. We handle phone-deduplication and role mapping automatically.</p>',
     TRUE, 30),
    ('payments', 'Do you take commission on payments?',
     '<p>Starter is 5%, Pro is 2%, Premium and Enterprise are <strong>0%</strong>. Razorpay''s gateway fee (~2%) is separate and charged by Razorpay directly to you.</p>',
     TRUE, 40),
    ('data', 'Who owns the student data?',
     '<p>You do. Premium tier ships database backups directly to your S3, Enterprise tier supports full self-hosting on your infra.</p>',
     TRUE, 50),
    ('migration', 'What happens if I outgrow you?',
     '<p>We export your data as a Postgres dump + MinIO bucket archive. Bring your own ops team or take it to any managed Postgres / S3 provider.</p>',
     TRUE, 60),
    ('billing', 'Is this billed yearly only?',
     '<p>Yes. Yearly billing keeps our infra costs predictable so we pass the savings to you. We do offer monthly contracts on Enterprise.</p>',
     FALSE, 100),
    ('tier-upgrade', 'Can I switch tiers later?',
     '<p>Yes — Starter → Pro upgrades happen instantly. Pro → Premium needs ~48h to compile your iOS app. We pro-rate the difference.</p>',
     FALSE, 110)
ON CONFLICT DO NOTHING;

-- Seed Privacy + Terms with the same content the static pages had so the
-- marketing site can switch over without a visible regression.
INSERT INTO cms_pages (slug, title, body_html, seo_title, seo_desc, is_published) VALUES
    ('privacy', 'Privacy Policy',
     '<p><strong>Vidya Warrior</strong> (the platform, accessible at vidyawarrior.com) is operated by Vidya Warrior Technologies Pvt Ltd from India. We sell a white-label coaching app to educators (tenants) who in turn serve their students (users).</p><p>Each tenant is the data controller for their students; we are the data processor on their behalf, plus the controller for tenant-level data (billing, support).</p><h2>What we collect</h2><ul><li>Phone number (primary identifier — required for OTP login)</li><li>Name and optional email</li><li>FCM device token (only if push enabled)</li><li>Course progress, watch history, attempt scores</li><li>Payment metadata from Razorpay (we do not store card details)</li><li>IP address + user agent on every request (for audit logs)</li></ul><h2>Who we share with</h2><p>Razorpay, MSG91, Gupshup, Firebase Cloud Messaging, Anthropic. We do not sell personal data to advertisers.</p><h2>How long we keep it</h2><p>Active accounts: as long as your tenant remains a customer. On account deletion: 30-day soft-delete window, then hard-delete from Postgres. Audit logs retained 7 years for tax compliance.</p><h2>Contact</h2><p>Email <a href="mailto:privacy@vidyawarrior.com">privacy@vidyawarrior.com</a>.</p>',
     'Privacy Policy · Vidya Warrior',
     'How Vidya Warrior handles personal data — students, instructors, and tenants.',
     TRUE),
    ('terms', 'Terms of Service',
     '<p>By signing up for Vidya Warrior you agree to these terms. They cover refund policy, content ownership, payment splits, suspension grounds.</p><h2>Refunds</h2><p>Yearly subscriptions are pro-rated for the unused period. Course-purchase refunds are at the tenant''s discretion within 7 days of purchase.</p><h2>Content ownership</h2><p>You retain ownership of all content you upload. We hold a non-exclusive license to host and serve it to your students.</p><h2>Payment splits</h2><p>Platform commission depends on your plan tier (5% Starter / 2% Pro / 0% Premium and Enterprise). Razorpay''s gateway fee is separate.</p><h2>Suspension</h2><p>Grounds: copyright violation, payment dispute, abuse of platform infrastructure.</p>',
     'Terms of Service · Vidya Warrior',
     'Terms governing use of Vidya Warrior (vidyawarrior.com).',
     TRUE)
ON CONFLICT (slug) DO UPDATE
    SET title     = EXCLUDED.title,
        body_html = EXCLUDED.body_html;
