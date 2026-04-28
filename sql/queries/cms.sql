-- name: ListPublishedPosts :many
SELECT id, slug, title, excerpt, cover_url, author_name, tags,
       published_at, minutes_read
FROM blog_posts
WHERE published_at IS NOT NULL AND published_at <= now()
ORDER BY published_at DESC
LIMIT $1 OFFSET $2;

-- name: GetPostBySlug :one
SELECT * FROM blog_posts
WHERE slug = $1 AND published_at IS NOT NULL AND published_at <= now();

-- name: AdminListPosts :many
-- Admin sees drafts too. No published filter, includes body_html length-only.
SELECT id, slug, title, excerpt, cover_url, author_name, tags,
       published_at, minutes_read, created_at, updated_at
FROM blog_posts
ORDER BY COALESCE(published_at, created_at) DESC
LIMIT $1 OFFSET $2;

-- name: AdminGetPostByID :one
SELECT * FROM blog_posts WHERE id = $1;

-- name: CreatePost :one
INSERT INTO blog_posts (
    slug, title, excerpt, body_json, body_html, cover_url,
    author_name, tags, published_at, minutes_read, seo_title, seo_desc, created_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: UpdatePost :one
UPDATE blog_posts
SET title        = COALESCE(NULLIF($2::text, ''),  title),
    excerpt      = COALESCE(NULLIF($3::text, ''),  excerpt),
    body_json    = COALESCE($4, body_json),
    body_html    = COALESCE(NULLIF($5::text, ''),  body_html),
    cover_url    = COALESCE(NULLIF($6::text, ''),  cover_url),
    author_name  = COALESCE(NULLIF($7::text, ''),  author_name),
    tags         = COALESCE($8, tags),
    published_at = $9,
    minutes_read = COALESCE(NULLIF($10, 0), minutes_read),
    seo_title    = COALESCE(NULLIF($11::text, ''), seo_title),
    seo_desc     = COALESCE(NULLIF($12::text, ''), seo_desc),
    updated_at   = now()
WHERE id = $1
RETURNING *;

-- name: DeletePost :exec
DELETE FROM blog_posts WHERE id = $1;

-- name: ListFaqs :many
-- Public read. Optional category filter; empty string returns all.
SELECT id, category, question, answer_html, show_on_home, display_order
FROM faqs
WHERE is_active = TRUE
  AND ($1::text = '' OR category = $1::text)
ORDER BY category, display_order;

-- name: ListHomepageFaqs :many
SELECT id, category, question, answer_html, display_order
FROM faqs
WHERE is_active = TRUE AND show_on_home = TRUE
ORDER BY display_order
LIMIT 8;

-- name: AdminListFaqs :many
SELECT * FROM faqs ORDER BY category, display_order;

-- name: CreateFaq :one
INSERT INTO faqs (category, question, answer_html, show_on_home, display_order)
VALUES ($1, $2, $3, COALESCE($4, FALSE), COALESCE(NULLIF($5, 0), 100))
RETURNING *;

-- name: UpdateFaq :one
UPDATE faqs
SET category      = COALESCE(NULLIF($2::text, ''), category),
    question      = COALESCE(NULLIF($3::text, ''), question),
    answer_html   = COALESCE(NULLIF($4::text, ''), answer_html),
    show_on_home  = COALESCE($5, show_on_home),
    is_active     = COALESCE($6, is_active),
    display_order = COALESCE(NULLIF($7, 0), display_order),
    updated_at    = now()
WHERE id = $1
RETURNING *;

-- name: DeleteFaq :exec
DELETE FROM faqs WHERE id = $1;

-- name: GetCmsPage :one
SELECT * FROM cms_pages WHERE slug = $1 AND is_published = TRUE;

-- name: AdminGetCmsPage :one
SELECT * FROM cms_pages WHERE slug = $1;

-- name: AdminListCmsPages :many
SELECT slug, title, is_published, updated_at FROM cms_pages ORDER BY slug;

-- name: UpsertCmsPage :one
INSERT INTO cms_pages (slug, title, body_json, body_html, seo_title, seo_desc, is_published, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, TRUE), now())
ON CONFLICT (slug) DO UPDATE
    SET title        = EXCLUDED.title,
        body_json    = EXCLUDED.body_json,
        body_html    = EXCLUDED.body_html,
        seo_title    = EXCLUDED.seo_title,
        seo_desc     = EXCLUDED.seo_desc,
        is_published = EXCLUDED.is_published,
        updated_at   = now()
RETURNING *;
