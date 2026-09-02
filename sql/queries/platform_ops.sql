-- platform_ops.sql — audit_logs, outbox, jobs, webhook_events, wallet,
-- payouts, referrals, cms/banners/leads. Worker-facing tables (outbox, jobs,
-- webhook_events) have no RLS; callers use WithSuperAdmin or write them
-- inside the request transaction.

-- ────────────────────────────────────────────────────────────── audit_logs

-- name: WriteAuditLog :exec
INSERT INTO audit_logs (tenant_id, actor_user_id, actor_role, action, entity_type, entity_id, before, after, ip, user_agent)
VALUES (sqlc.narg(tenant_id)::uuid, sqlc.narg(actor_user_id)::uuid, sqlc.narg(actor_role)::text,
        $1, sqlc.narg(entity_type)::text, sqlc.narg(entity_id)::uuid,
        sqlc.narg(before)::jsonb, sqlc.narg(after)::jsonb, sqlc.narg(ip)::inet, sqlc.narg(user_agent)::text);

-- name: ListAuditLogs :many
SELECT id, actor_user_id, actor_role, action, entity_type, entity_id, created_at
FROM audit_logs
WHERE (sqlc.narg(tenant_id)::uuid IS NULL OR tenant_id = sqlc.narg(tenant_id)::uuid)
ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- ────────────────────────────────────────────────────────────────── outbox

-- name: EnqueueOutbox :exec
INSERT INTO outbox (aggregate_type, aggregate_id, event_type, tenant_id, payload)
VALUES ($1, $2, $3, sqlc.narg(tenant_id)::uuid, $4);

-- name: ClaimOutboxBatch :many
SELECT id, aggregate_type, aggregate_id, event_type, tenant_id, payload, attempts
FROM outbox WHERE published_at IS NULL
ORDER BY id LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxPublished :exec
UPDATE outbox SET published_at = now() WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: MarkOutboxFailed :exec
UPDATE outbox SET attempts = attempts + 1, last_error = $2 WHERE id = $1;

-- ─────────────────────────────────────────────────────────────────── jobs

-- name: EnqueueJob :one
INSERT INTO jobs (kind, tenant_id, payload, run_after)
VALUES ($1, sqlc.narg(tenant_id)::uuid, COALESCE(sqlc.narg(payload)::jsonb, '{}'::jsonb),
        COALESCE(sqlc.narg(run_after)::timestamptz, now()))
RETURNING id, kind, status, run_after;

-- name: ClaimJobs :many
UPDATE jobs SET status = 'running', locked_at = now(), locked_by = $2, attempts = attempts + 1
WHERE id IN (
    SELECT id FROM jobs
    WHERE status = 'pending' AND run_after <= now()
    ORDER BY run_after LIMIT $1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, kind, tenant_id, payload, attempts, max_attempts;

-- name: CompleteJob :exec
UPDATE jobs SET status = 'done', completed_at = now() WHERE id = $1;

-- name: FailJob :exec
UPDATE jobs SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
    last_error = $2, locked_at = NULL, locked_by = NULL,
    run_after = now() + interval '1 minute' * attempts
WHERE id = $1;

-- ───────────────────────────────────────────────────────────── webhook_events

-- name: RecordWebhookEvent :one
INSERT INTO webhook_events (gateway, event_id, event_type, payload, signature_ok)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (gateway, event_id) DO NOTHING
RETURNING id, gateway, event_id, event_type;

-- name: MarkWebhookProcessed :exec
UPDATE webhook_events SET processed_at = now(), process_error = sqlc.narg(process_error)::text
WHERE gateway = $1 AND event_id = $2;

-- ──────────────────────────────────────────────────────────────── wallet

-- name: GetOrCreateWallet :one
INSERT INTO wallets (tenant_id, user_id) VALUES ($1, $2)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET updated_at = now()
RETURNING id, tenant_id, user_id, balance_minor, currency;

-- name: GetWalletForUpdate :one
SELECT id, balance_minor FROM wallets WHERE tenant_id = $1 AND user_id = $2 FOR UPDATE;

-- name: ApplyWalletTransaction :one
WITH upd AS (
    UPDATE wallets SET balance_minor = balance_minor + sqlc.arg(amount_minor)::bigint
    WHERE id = sqlc.arg(wallet_id)::uuid
    RETURNING balance_minor
)
INSERT INTO wallet_transactions (tenant_id, wallet_id, user_id, amount_minor, kind, ref_type, ref_id, balance_after_minor, note)
SELECT $1, sqlc.arg(wallet_id)::uuid, $2, sqlc.arg(amount_minor)::bigint, $3,
       sqlc.narg(ref_type)::text, sqlc.narg(ref_id)::uuid, upd.balance_minor, sqlc.narg(note)::text
FROM upd
RETURNING id, amount_minor, kind, balance_after_minor, created_at;

-- name: ListWalletTransactions :many
SELECT amount_minor, kind, ref_type, ref_id, balance_after_minor, note, created_at
FROM wallet_transactions WHERE tenant_id = $1 AND user_id = $2
ORDER BY created_at DESC LIMIT $3 OFFSET $4;

-- ──────────────────────────────────────────────────────────────── payouts

-- name: CreatePayout :one
INSERT INTO payouts (tenant_id, payee_user_id, kind, amount_minor, tds_minor, net_minor, method, note)
VALUES ($1, $2, $3, $4, COALESCE(sqlc.narg(tds_minor)::bigint, 0), $5, sqlc.narg(method)::text, sqlc.narg(note)::text)
RETURNING id, payee_user_id, kind, amount_minor, tds_minor, net_minor, status, requested_at;

-- name: AddPayoutItem :exec
INSERT INTO payout_items (tenant_id, payout_id, ref_type, ref_id, amount_minor)
VALUES ($1, $2, $3, $4, $5);

-- name: SetPayoutStatus :exec
UPDATE payouts SET status = $2, gateway_payout_id = sqlc.narg(gateway_payout_id)::text,
    processed_at = CASE WHEN $2 = 'paid'::payout_status THEN now() ELSE processed_at END
WHERE id = $1;

-- name: ListPayouts :many
SELECT id, payee_user_id, kind, amount_minor, net_minor, status, requested_at, processed_at
FROM payouts WHERE tenant_id = $1
  AND (sqlc.narg(status)::payout_status IS NULL OR status = sqlc.narg(status)::payout_status)
ORDER BY requested_at DESC LIMIT $2 OFFSET $3;

-- ──────────────────────────────────────────────────────────────── referrals

-- name: GetOrCreateReferralCode :one
INSERT INTO referral_codes (tenant_id, user_id, code) VALUES ($1, $2, sqlc.arg(code)::citext)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET updated_at = now()
RETURNING id, tenant_id, user_id, code, uses;

-- name: GetReferralCodeByCode :one
SELECT id, tenant_id, user_id, code, uses FROM referral_codes WHERE code = sqlc.arg(code)::citext;

-- name: IncrementReferralCodeUses :exec
UPDATE referral_codes SET uses = uses + 1 WHERE id = $1;

-- name: CreateReferralEvent :one
INSERT INTO referral_events (tenant_id, code, referrer_user_id, referred_user_id, status)
VALUES ($1, sqlc.arg(code)::citext, sqlc.narg(referrer_user_id)::uuid, $2, 'pending')
ON CONFLICT (referred_user_id) DO NOTHING
RETURNING id, tenant_id, code, referrer_user_id, referred_user_id, status;

-- name: GetPendingReferralForUser :one
SELECT id, tenant_id, code, referrer_user_id, referred_user_id, status, reward_minor
FROM referral_events WHERE referred_user_id = $1 AND status = 'pending';

-- name: MarkReferralRewarded :exec
UPDATE referral_events
SET status = 'rewarded', reward_minor = $2, qualifying_order_id = sqlc.narg(qualifying_order_id)::uuid,
    wallet_transaction_id = sqlc.narg(wallet_transaction_id)::uuid, rewarded_at = now()
WHERE id = $1 AND status = 'pending';

-- name: ReferralStatsForUser :one
SELECT count(*) FILTER (WHERE status = 'rewarded') AS rewarded,
       count(*)                                    AS total,
       COALESCE(sum(reward_minor) FILTER (WHERE status = 'rewarded'), 0)::bigint AS total_reward_minor
FROM referral_events WHERE tenant_id = $1 AND referrer_user_id = $2;

-- ───────────────────────────────────────────────────────────── cms / banners / leads

-- name: ListBanners :many
SELECT id, title, subtitle, image_url, background_color, link_type, link_id, link_url, display_order
FROM banners
WHERE tenant_id = $1 AND is_active
  AND (starts_at IS NULL OR starts_at <= now()) AND (ends_at IS NULL OR ends_at > now())
ORDER BY display_order;

-- name: CreateBanner :one
INSERT INTO banners (tenant_id, title, subtitle, image_url, background_color, link_type, link_id, link_url, display_order, starts_at, ends_at, created_by)
VALUES ($1, $2, sqlc.narg(subtitle)::text, $3, sqlc.narg(background_color)::text,
        sqlc.narg(link_type)::text, sqlc.narg(link_id)::uuid, sqlc.narg(link_url)::text,
        COALESCE(sqlc.narg(display_order)::int, 0), sqlc.narg(starts_at)::timestamptz,
        sqlc.narg(ends_at)::timestamptz, sqlc.narg(created_by)::uuid)
RETURNING id, title, image_url, display_order, is_active;

-- name: SetBannerActive :exec
UPDATE banners SET is_active = $2 WHERE id = $1;

-- name: DeleteBanner :exec
DELETE FROM banners WHERE id = $1;

-- name: ListAllBanners :many
SELECT id, title, subtitle, image_url, background_color, link_type, link_id,
       link_url, display_order, is_active, starts_at, ends_at, created_at
FROM banners WHERE tenant_id = $1
ORDER BY display_order, created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateBanner :one
UPDATE banners SET
    title            = COALESCE(sqlc.narg(title)::text, title),
    subtitle         = COALESCE(sqlc.narg(subtitle)::text, subtitle),
    image_url        = COALESCE(sqlc.narg(image_url)::text, image_url),
    background_color = COALESCE(sqlc.narg(background_color)::text, background_color),
    link_type        = COALESCE(sqlc.narg(link_type)::text, link_type),
    link_id          = COALESCE(sqlc.narg(link_id)::uuid, link_id),
    link_url         = COALESCE(sqlc.narg(link_url)::text, link_url),
    display_order    = COALESCE(sqlc.narg(display_order)::int, display_order),
    is_active        = COALESCE(sqlc.narg(is_active)::boolean, is_active),
    starts_at        = COALESCE(sqlc.narg(starts_at)::timestamptz, starts_at),
    ends_at          = COALESCE(sqlc.narg(ends_at)::timestamptz, ends_at)
WHERE id = $1
RETURNING id, title, subtitle, image_url, background_color, link_type, link_id,
          link_url, display_order, is_active, starts_at, ends_at, created_at;

-- name: ListBlogPosts :many
SELECT id, slug, title, excerpt, cover_url, author_name, tags, minutes_read, published_at
FROM blog_posts WHERE published_at IS NOT NULL AND published_at <= now()
ORDER BY published_at DESC LIMIT $1 OFFSET $2;

-- name: GetBlogPost :one
SELECT id, slug, title, excerpt, body_json, body_html, cover_url, author_name, tags, minutes_read, published_at
FROM blog_posts WHERE slug = sqlc.arg(slug)::citext;

-- name: UpsertBlogPost :one
INSERT INTO blog_posts (slug, title, excerpt, body_json, body_html, cover_url, author_name, tags, minutes_read, seo_title, seo_desc, published_at, created_by)
VALUES (sqlc.arg(slug)::citext, $1, sqlc.narg(excerpt)::text,
        COALESCE(sqlc.narg(body_json)::jsonb, '{}'::jsonb), COALESCE(sqlc.narg(body_html)::text, ''),
        sqlc.narg(cover_url)::text, sqlc.narg(author_name)::text, COALESCE(sqlc.narg(tags)::text[], '{}'),
        COALESCE(sqlc.narg(minutes_read)::int, 3), sqlc.narg(seo_title)::text, sqlc.narg(seo_desc)::text,
        sqlc.narg(published_at)::timestamptz, sqlc.narg(created_by)::uuid)
ON CONFLICT (slug) DO UPDATE SET
    title = EXCLUDED.title, excerpt = EXCLUDED.excerpt, body_json = EXCLUDED.body_json,
    body_html = EXCLUDED.body_html, cover_url = EXCLUDED.cover_url, tags = EXCLUDED.tags,
    published_at = EXCLUDED.published_at
RETURNING id, slug, title, published_at;

-- name: ListFaqs :many
SELECT id, category, question, answer_html, show_on_home, display_order
FROM faqs WHERE is_active
  AND (sqlc.narg(category)::text IS NULL OR category = sqlc.narg(category)::text)
ORDER BY category, display_order;

-- name: GetCmsPage :one
SELECT slug, title, body_json, body_html, seo_title, seo_desc FROM cms_pages
WHERE slug = sqlc.arg(slug)::citext AND is_published;

-- name: UpsertCmsPage :exec
INSERT INTO cms_pages (slug, title, body_json, body_html, seo_title, seo_desc, is_published)
VALUES (sqlc.arg(slug)::citext, $1, COALESCE(sqlc.narg(body_json)::jsonb, '{}'::jsonb),
        COALESCE(sqlc.narg(body_html)::text, ''), sqlc.narg(seo_title)::text,
        sqlc.narg(seo_desc)::text, COALESCE(sqlc.narg(is_published)::boolean, true))
ON CONFLICT (slug) DO UPDATE SET title = EXCLUDED.title, body_json = EXCLUDED.body_json,
    body_html = EXCLUDED.body_html, is_published = EXCLUDED.is_published;

-- name: CreateLead :one
INSERT INTO leads (name, phone, email, institute_name, city, students_count, source, utm)
VALUES (sqlc.narg(name)::text, sqlc.narg(phone)::citext, sqlc.narg(email)::citext,
        sqlc.narg(institute_name)::text, sqlc.narg(city)::text, sqlc.narg(students_count)::int,
        sqlc.narg(source)::text, COALESCE(sqlc.narg(utm)::jsonb, '{}'::jsonb))
RETURNING id, name, phone, email, status, created_at;

-- name: ListLeads :many
SELECT id, name, phone, email, institute_name, city, students_count, source, status, assigned_to, created_at
FROM leads
WHERE (sqlc.narg(status)::lead_status IS NULL OR status = sqlc.narg(status)::lead_status)
ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: UpdateLeadStatus :exec
UPDATE leads SET status = $2, assigned_to = sqlc.narg(assigned_to)::uuid, notes = sqlc.narg(notes)::text
WHERE id = $1;

-- ─────────────────────────────────────────────────────── cms admin (super)

-- name: AdminListBlogPosts :many
SELECT id, slug, title, excerpt, cover_url, author_name, tags, minutes_read, published_at
FROM blog_posts ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: GetBlogPostByID :one
SELECT id, slug, title, excerpt, body_json, body_html, cover_url, author_name, tags,
       minutes_read, seo_title, seo_desc, published_at
FROM blog_posts WHERE id = $1;

-- name: DeleteBlogPost :exec
DELETE FROM blog_posts WHERE id = $1;

-- name: AdminListFaqs :many
SELECT id, category, question, answer_html, show_on_home, is_active, display_order
FROM faqs ORDER BY category, display_order;

-- name: CreateFaq :one
INSERT INTO faqs (category, question, answer_html, show_on_home, display_order)
VALUES (COALESCE(sqlc.narg(category)::text, 'general'), $1, $2,
        COALESCE(sqlc.narg(show_on_home)::boolean, false),
        COALESCE(sqlc.narg(display_order)::int, 0))
RETURNING id, category, question, answer_html, show_on_home, is_active, display_order;

-- name: UpdateFaq :one
UPDATE faqs SET
    category      = COALESCE(sqlc.narg(category)::text, category),
    question      = COALESCE(sqlc.narg(question)::text, question),
    answer_html   = COALESCE(sqlc.narg(answer_html)::text, answer_html),
    show_on_home  = COALESCE(sqlc.narg(show_on_home)::boolean, show_on_home),
    is_active     = COALESCE(sqlc.narg(is_active)::boolean, is_active),
    display_order = COALESCE(sqlc.narg(display_order)::int, display_order)
WHERE id = $1
RETURNING id, category, question, answer_html, show_on_home, is_active, display_order;

-- name: DeleteFaq :exec
DELETE FROM faqs WHERE id = $1;

-- name: AdminListCmsPages :many
SELECT slug, title, is_published, updated_at FROM cms_pages ORDER BY slug;

-- name: AdminGetCmsPage :one
SELECT slug, title, body_json, body_html, seo_title, seo_desc, is_published
FROM cms_pages WHERE slug = sqlc.arg(slug)::citext;
