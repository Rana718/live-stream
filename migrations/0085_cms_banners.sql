-- 0085_cms_banners.sql
-- Tenant home-screen banners (tenant-scoped) and platform marketing content
-- (blog posts, FAQs, free-form pages — not tenant-scoped, publicly readable).

-- ─────────────────────────────────────────────────────────────────── banners
CREATE TABLE banners (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    title            text NOT NULL,
    subtitle         text,
    image_url        text NOT NULL,
    background_color text,
    link_type        text CHECK (link_type IS NULL OR link_type IN ('course','bundle','test','url','none')),
    link_id          uuid,
    link_url         text,
    display_order    integer NOT NULL DEFAULT 0,
    is_active        boolean NOT NULL DEFAULT true,
    starts_at        timestamptz,
    ends_at          timestamptz,
    created_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_banners_tenant_active ON banners (tenant_id, is_active, display_order);
SELECT apply_tenant_table('banners');

-- ─────────────────────────────────────────────────────────── blog_posts
CREATE TABLE blog_posts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         citext NOT NULL UNIQUE,
    title        text NOT NULL,
    excerpt      text,
    body_json    jsonb NOT NULL DEFAULT '{}'::jsonb,
    body_html    text NOT NULL DEFAULT '',
    cover_url    text,
    author_name  text,
    tags         text[] NOT NULL DEFAULT '{}',
    minutes_read integer NOT NULL DEFAULT 3,
    seo_title    text,
    seo_desc     text,
    published_at timestamptz,
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_blog_posts_published ON blog_posts (published_at DESC NULLS LAST);
CREATE INDEX idx_blog_posts_tags ON blog_posts USING gin (tags);
ALTER TABLE blog_posts ENABLE ROW LEVEL SECURITY;
ALTER TABLE blog_posts FORCE ROW LEVEL SECURITY;
CREATE POLICY blog_posts_read ON blog_posts FOR SELECT USING (true);
CREATE POLICY blog_posts_super ON blog_posts USING (is_super_admin()) WITH CHECK (is_super_admin());
SELECT apply_updated_at('blog_posts');
SELECT apply_app_grants('blog_posts');

-- ──────────────────────────────────────────────────────────────────── faqs
CREATE TABLE faqs (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    category      text NOT NULL DEFAULT 'general',
    question      text NOT NULL,
    answer_html   text NOT NULL,
    show_on_home  boolean NOT NULL DEFAULT false,
    is_active     boolean NOT NULL DEFAULT true,
    display_order integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_faqs_category ON faqs (category, display_order) WHERE is_active;
ALTER TABLE faqs ENABLE ROW LEVEL SECURITY;
ALTER TABLE faqs FORCE ROW LEVEL SECURITY;
CREATE POLICY faqs_read ON faqs FOR SELECT USING (true);
CREATE POLICY faqs_super ON faqs USING (is_super_admin()) WITH CHECK (is_super_admin());
SELECT apply_updated_at('faqs');
SELECT apply_app_grants('faqs');

-- ─────────────────────────────────────────────────────────────── cms_pages
CREATE TABLE cms_pages (
    slug         citext PRIMARY KEY,
    title        text NOT NULL,
    body_json    jsonb NOT NULL DEFAULT '{}'::jsonb,
    body_html    text NOT NULL DEFAULT '',
    seo_title    text,
    seo_desc     text,
    is_published boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE cms_pages ENABLE ROW LEVEL SECURITY;
ALTER TABLE cms_pages FORCE ROW LEVEL SECURITY;
CREATE POLICY cms_pages_read ON cms_pages FOR SELECT USING (true);
CREATE POLICY cms_pages_super ON cms_pages USING (is_super_admin()) WITH CHECK (is_super_admin());
SELECT apply_updated_at('cms_pages');
SELECT apply_app_grants('cms_pages');
