-- engagement.sql — reviews, forum, course chat, badges, streaks, wishlists,
-- gifts. All under standard tenant RLS + sqlc (v1 had these on raw pgx).

-- name: UpsertCourseReview :one
INSERT INTO course_reviews (tenant_id, course_id, user_id, rating, body)
VALUES ($1, $2, $3, $4, COALESCE(sqlc.narg(body)::text, ''))
ON CONFLICT (course_id, user_id) DO UPDATE SET rating = EXCLUDED.rating, body = EXCLUDED.body
RETURNING id, course_id, user_id, rating, body, is_approved, created_at;

-- name: ListCourseReviews :many
SELECT r.id, r.rating, r.body, r.created_at, u.full_name, u.avatar_url
FROM course_reviews r JOIN users u ON u.id = r.user_id
WHERE r.tenant_id = $1 AND r.course_id = $2 AND r.is_approved
ORDER BY r.created_at DESC LIMIT $3 OFFSET $4;

-- name: CourseRatingSummary :one
SELECT COALESCE(avg(rating), 0)::numeric AS avg_rating, count(*) AS total
FROM course_reviews WHERE tenant_id = $1 AND course_id = $2 AND is_approved;

-- name: SetReviewApproved :exec
UPDATE course_reviews SET is_approved = $2 WHERE id = $1;

-- name: CreateForumThread :one
INSERT INTO forum_threads (tenant_id, course_id, user_id, title, body)
VALUES ($1, sqlc.narg(course_id)::uuid, $2, $3, COALESCE(sqlc.narg(body)::text, ''))
RETURNING id, course_id, user_id, title, body, created_at;

-- name: ListForumThreads :many
SELECT t.id, t.title, t.is_pinned, t.is_locked, t.reply_count, t.last_reply_at, t.created_at, u.full_name
FROM forum_threads t JOIN users u ON u.id = t.user_id
WHERE t.tenant_id = $1
  AND (sqlc.narg(course_id)::uuid IS NULL OR t.course_id = sqlc.narg(course_id)::uuid)
ORDER BY t.is_pinned DESC, t.last_reply_at DESC NULLS LAST LIMIT $2 OFFSET $3;

-- name: GetForumThread :one
SELECT id, tenant_id, course_id, user_id, title, body, is_locked, reply_count, created_at
FROM forum_threads WHERE id = $1;

-- name: AddForumPost :one
INSERT INTO forum_posts (tenant_id, thread_id, user_id, body)
VALUES ($1, $2, $3, $4)
RETURNING id, thread_id, user_id, body, is_instructor_highlight, created_at;

-- name: BumpForumThread :exec
UPDATE forum_threads SET reply_count = reply_count + 1, last_reply_at = now() WHERE id = $1;

-- name: ListForumPosts :many
SELECT p.id, p.body, p.is_instructor_highlight, p.created_at, u.full_name, u.avatar_url
FROM forum_posts p JOIN users u ON u.id = p.user_id
WHERE p.thread_id = $1 ORDER BY p.created_at LIMIT $2 OFFSET $3;

-- name: HighlightForumPost :exec
UPDATE forum_posts SET is_instructor_highlight = $2 WHERE id = $1;

-- name: PostCourseChat :one
INSERT INTO course_chat_messages (tenant_id, course_id, user_id, body) VALUES ($1, $2, $3, $4)
RETURNING id, course_id, user_id, body, created_at;

-- name: ListCourseChat :many
SELECT m.id, m.body, m.created_at, u.full_name, u.avatar_url
FROM course_chat_messages m JOIN users u ON u.id = m.user_id
WHERE m.tenant_id = $1 AND m.course_id = $2
ORDER BY m.created_at DESC LIMIT $3 OFFSET $4;

-- name: ListBadges :many
SELECT id, code, name, description, icon, points FROM badges ORDER BY points;

-- name: GrantBadge :exec
INSERT INTO badge_grants (tenant_id, user_id, badge_id)
VALUES ($1, $2, $3) ON CONFLICT (tenant_id, user_id, badge_id) DO NOTHING;

-- name: ListUserBadges :many
SELECT b.code, b.name, b.icon, b.points, g.earned_at
FROM badge_grants g JOIN badges b ON b.id = g.badge_id
WHERE g.tenant_id = $1 AND g.user_id = $2 ORDER BY g.earned_at DESC;

-- name: UpsertLearningStreak :one
INSERT INTO learning_streaks (tenant_id, user_id, last_active_date, current_streak, longest_streak, total_points)
VALUES ($1, $2, current_date, 1, 1, sqlc.narg(points)::int)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    current_streak = CASE
        WHEN learning_streaks.last_active_date = current_date THEN learning_streaks.current_streak
        WHEN learning_streaks.last_active_date = current_date - 1 THEN learning_streaks.current_streak + 1
        ELSE 1 END,
    longest_streak = GREATEST(learning_streaks.longest_streak,
        CASE WHEN learning_streaks.last_active_date = current_date - 1 THEN learning_streaks.current_streak + 1 ELSE 1 END),
    total_points = learning_streaks.total_points + COALESCE(sqlc.narg(points)::int, 0),
    last_active_date = current_date
RETURNING current_streak, longest_streak, total_points;

-- name: GetLearningStreak :one
SELECT current_streak, longest_streak, total_points, last_active_date
FROM learning_streaks WHERE tenant_id = $1 AND user_id = $2;

-- name: AddToWishlist :exec
INSERT INTO wishlists (tenant_id, user_id, course_id) VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, user_id, course_id) DO NOTHING;

-- name: RemoveFromWishlist :exec
DELETE FROM wishlists WHERE tenant_id = $1 AND user_id = $2 AND course_id = $3;

-- name: ListWishlist :many
SELECT w.course_id, c.title, c.slug, c.thumbnail_url, w.created_at
FROM wishlists w JOIN courses c ON c.id = w.course_id
WHERE w.tenant_id = $1 AND w.user_id = $2 ORDER BY w.created_at DESC;

-- name: CreateCourseGift :one
INSERT INTO course_gifts (tenant_id, sender_id, order_id, product_id, recipient_phone, recipient_email, redemption_code, message)
VALUES ($1, $2, sqlc.narg(order_id)::uuid, sqlc.narg(product_id)::uuid,
        sqlc.narg(recipient_phone)::citext, sqlc.narg(recipient_email)::citext, $3, sqlc.narg(message)::text)
RETURNING id, redemption_code, recipient_phone, recipient_email, created_at;

-- name: RedeemCourseGift :one
UPDATE course_gifts SET redeemed_by = $2, redeemed_at = now(), entitlement_id = sqlc.narg(entitlement_id)::uuid
WHERE redemption_code = $1 AND redeemed_at IS NULL
RETURNING id, tenant_id, sender_id, product_id, redeemed_by;
