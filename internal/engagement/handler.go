// Package engagement bundles the lighter "social" features into one
// place: course reviews, forum, gamification, wishlist, gifts, lecture
// notes, course chat. Each handler is small enough that splitting into
// per-feature packages would just churn imports for no benefit.
//
// All queries hit the pgx pool directly — sqlc regen is locked because
// the generated db files in /internal/database/db are root-owned.
package engagement

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool *pgxpool.Pool
}

func NewHandler(pool *pgxpool.Pool) *Handler { return &Handler{pool: pool} }

// ──────────────────────────────────────────────────────────── helpers

func ctxIDs(c fiber.Ctx) (uuid.UUID, uuid.UUID) {
	uid, _ := c.Locals("userID").(uuid.UUID)
	tid, _ := c.Locals("tenantID").(uuid.UUID)
	return uid, tid
}

func ctxRole(c fiber.Ctx) string {
	r, _ := c.Locals("userRole").(string)
	return r
}

func bad(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": msg})
}

func ise(c fiber.Ctx, err error) error {
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
}

func randCode(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func parseUUID(c fiber.Ctx, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Params(key))
	if err != nil {
		_ = bad(c, fmt.Sprintf("invalid %s", key))
		return uuid.Nil, false
	}
	return id, true
}

func scanRows(rows pgx.Rows, fields ...string) ([]map[string]any, error) {
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		m := map[string]any{}
		for i, f := range fields {
			if i < len(vals) {
				m[f] = vals[i]
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ──────────────────────────────────────────────────────────── REVIEWS

func (h *Handler) ListReviews(c fiber.Ctx) error {
	courseID, ok := parseUUID(c, "id")
	if !ok {
		return nil
	}
	rows, err := h.pool.Query(context.Background(), `
		SELECT r.id, r.rating, r.body, r.created_at,
		       u.full_name, u.id AS user_id
		FROM course_reviews r
		JOIN users u ON u.id = r.user_id
		WHERE r.course_id = $1 AND r.is_approved = TRUE
		ORDER BY r.created_at DESC
		LIMIT 200
	`, courseID)
	if err != nil {
		return ise(c, err)
	}
	out, err := scanRows(rows, "id", "rating", "body", "created_at", "full_name", "user_id")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(out)
}

// ReviewSummary returns the rating histogram for a course (used by the
// public course detail page so you don't have to count rows client-side).
func (h *Handler) ReviewSummary(c fiber.Ctx) error {
	courseID, ok := parseUUID(c, "id")
	if !ok {
		return nil
	}
	row := h.pool.QueryRow(context.Background(), `
		SELECT COALESCE(AVG(rating), 0)::float, COUNT(*)::int,
		       COUNT(*) FILTER (WHERE rating = 5)::int,
		       COUNT(*) FILTER (WHERE rating = 4)::int,
		       COUNT(*) FILTER (WHERE rating = 3)::int,
		       COUNT(*) FILTER (WHERE rating = 2)::int,
		       COUNT(*) FILTER (WHERE rating = 1)::int
		FROM course_reviews
		WHERE course_id = $1 AND is_approved = TRUE
	`, courseID)
	var avg float64
	var n, r5, r4, r3, r2, r1 int
	if err := row.Scan(&avg, &n, &r5, &r4, &r3, &r2, &r1); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{
		"average": avg, "count": n,
		"buckets": fiber.Map{"5": r5, "4": r4, "3": r3, "2": r2, "1": r1},
	})
}

func (h *Handler) UpsertReview(c fiber.Ctx) error {
	courseID, ok := parseUUID(c, "id")
	if !ok {
		return nil
	}
	uid, tid := ctxIDs(c)
	var body struct {
		Rating int    `json:"rating"`
		Body   string `json:"body"`
	}
	if err := c.Bind().Body(&body); err != nil || body.Rating < 1 || body.Rating > 5 {
		return bad(c, "rating must be 1..5")
	}
	_, err := h.pool.Exec(context.Background(), `
		INSERT INTO course_reviews (tenant_id, course_id, user_id, rating, body)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (course_id, user_id) DO UPDATE
		   SET rating = EXCLUDED.rating, body = EXCLUDED.body
	`, tid, courseID, uid, body.Rating, body.Body)
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"saved": true})
}

func (h *Handler) AdminListReviews(c fiber.Ctx) error {
	rows, err := h.pool.Query(context.Background(), `
		SELECT r.id, r.rating, r.body, r.is_approved, r.created_at,
		       u.full_name, c.title
		FROM course_reviews r
		JOIN users u ON u.id = r.user_id
		JOIN courses c ON c.id = r.course_id
		ORDER BY r.created_at DESC
		LIMIT 500
	`)
	if err != nil {
		return ise(c, err)
	}
	out, err := scanRows(rows, "id", "rating", "body", "is_approved", "created_at", "full_name", "course_title")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) AdminSetReviewApproved(c fiber.Ctx) error {
	id, ok := parseUUID(c, "id")
	if !ok {
		return nil
	}
	var body struct {
		Approved bool `json:"approved"`
	}
	_ = c.Bind().Body(&body)
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE course_reviews SET is_approved = $2 WHERE id = $1`, id, body.Approved); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"updated": true})
}

func (h *Handler) AdminDeleteReview(c fiber.Ctx) error {
	id, ok := parseUUID(c, "id")
	if !ok {
		return nil
	}
	if _, err := h.pool.Exec(context.Background(),
		`DELETE FROM course_reviews WHERE id = $1`, id); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"deleted": true})
}

// ──────────────────────────────────────────────────────────── FORUM

func (h *Handler) ListThreads(c fiber.Ctx) error {
	courseID := c.Query("course_id")
	args := []any{}
	q := `SELECT t.id, t.title, t.body, t.is_pinned, t.is_locked, t.reply_count,
	             t.last_reply_at, t.created_at, u.full_name, t.user_id, t.course_id
	      FROM forum_threads t
	      JOIN users u ON u.id = t.user_id`
	if courseID != "" {
		q += ` WHERE t.course_id = $1`
		args = append(args, courseID)
	}
	q += ` ORDER BY t.is_pinned DESC, COALESCE(t.last_reply_at, t.created_at) DESC LIMIT 200`
	rows, err := h.pool.Query(context.Background(), q, args...)
	if err != nil {
		return ise(c, err)
	}
	out, err := scanRows(rows, "id", "title", "body", "is_pinned", "is_locked",
		"reply_count", "last_reply_at", "created_at", "full_name", "user_id", "course_id")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) CreateThread(c fiber.Ctx) error {
	uid, tid := ctxIDs(c)
	var body struct {
		CourseID string `json:"course_id"`
		Title    string `json:"title"`
		Body     string `json:"body"`
	}
	if err := c.Bind().Body(&body); err != nil || strings.TrimSpace(body.Title) == "" {
		return bad(c, "title required")
	}
	var threadID uuid.UUID
	var courseID any
	if body.CourseID != "" {
		if cid, err := uuid.Parse(body.CourseID); err == nil {
			courseID = cid
		}
	}
	row := h.pool.QueryRow(context.Background(), `
		INSERT INTO forum_threads (tenant_id, course_id, user_id, title, body)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, tid, courseID, uid, body.Title, body.Body)
	if err := row.Scan(&threadID); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"id": threadID})
}

func (h *Handler) ListPosts(c fiber.Ctx) error {
	threadID, ok := parseUUID(c, "id")
	if !ok {
		return nil
	}
	rows, err := h.pool.Query(context.Background(), `
		SELECT p.id, p.body, p.is_instructor_highlight, p.created_at,
		       u.full_name, p.user_id
		FROM forum_posts p
		JOIN users u ON u.id = p.user_id
		WHERE p.thread_id = $1
		ORDER BY p.created_at ASC
	`, threadID)
	if err != nil {
		return ise(c, err)
	}
	out, err := scanRows(rows, "id", "body", "is_instructor_highlight", "created_at", "full_name", "user_id")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) CreatePost(c fiber.Ctx) error {
	threadID, ok := parseUUID(c, "id")
	if !ok {
		return nil
	}
	uid, tid := ctxIDs(c)
	var body struct {
		Body string `json:"body"`
	}
	if err := c.Bind().Body(&body); err != nil || strings.TrimSpace(body.Body) == "" {
		return bad(c, "body required")
	}
	role := ctxRole(c)
	highlight := role == "instructor" || role == "admin" || role == "super_admin"
	tx, err := h.pool.Begin(context.Background())
	if err != nil {
		return ise(c, err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(context.Background(), `
		INSERT INTO forum_posts (tenant_id, thread_id, user_id, body, is_instructor_highlight)
		VALUES ($1, $2, $3, $4, $5)
	`, tid, threadID, uid, body.Body, highlight); err != nil {
		return ise(c, err)
	}
	if _, err := tx.Exec(context.Background(), `
		UPDATE forum_threads
		   SET reply_count = reply_count + 1, last_reply_at = now()
		 WHERE id = $1
	`, threadID); err != nil {
		return ise(c, err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"posted": true})
}

func (h *Handler) AdminPinThread(c fiber.Ctx) error {
	id, ok := parseUUID(c, "id")
	if !ok {
		return nil
	}
	var body struct {
		Pinned bool `json:"pinned"`
	}
	_ = c.Bind().Body(&body)
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE forum_threads SET is_pinned = $2 WHERE id = $1`, id, body.Pinned); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"updated": true})
}

func (h *Handler) AdminLockThread(c fiber.Ctx) error {
	id, ok := parseUUID(c, "id")
	if !ok {
		return nil
	}
	var body struct {
		Locked bool `json:"locked"`
	}
	_ = c.Bind().Body(&body)
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE forum_threads SET is_locked = $2 WHERE id = $1`, id, body.Locked); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"updated": true})
}

// ──────────────────────────────────────────────────────────── GAMIFICATION

func (h *Handler) ListBadges(c fiber.Ctx) error {
	rows, err := h.pool.Query(context.Background(),
		`SELECT id, code, name, description, icon, points FROM badges ORDER BY points`)
	if err != nil {
		return ise(c, err)
	}
	out, err := scanRows(rows, "id", "code", "name", "description", "icon", "points")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) MyGamification(c fiber.Ctx) error {
	uid, _ := ctxIDs(c)
	row := h.pool.QueryRow(context.Background(), `
		SELECT COALESCE(current_streak, 0), COALESCE(longest_streak, 0), COALESCE(total_points, 0),
		       last_active_date
		FROM user_streaks WHERE user_id = $1
	`, uid)
	var cur, longest, points int
	var last *time.Time
	if err := row.Scan(&cur, &longest, &points, &last); err != nil && err != pgx.ErrNoRows {
		return ise(c, err)
	}
	bRows, err := h.pool.Query(context.Background(), `
		SELECT b.id, b.code, b.name, b.icon, b.points, g.earned_at
		FROM badge_grants g JOIN badges b ON b.id = g.badge_id
		WHERE g.user_id = $1 ORDER BY g.earned_at DESC
	`, uid)
	if err != nil {
		return ise(c, err)
	}
	badges, err := scanRows(bRows, "id", "code", "name", "icon", "points", "earned_at")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{
		"current_streak":   cur,
		"longest_streak":   longest,
		"total_points":     points,
		"last_active_date": last,
		"badges":           badges,
	})
}

// CheckIn extends the user's daily streak. Idempotent for the day —
// calling twice on the same date doesn't double-count.
func (h *Handler) CheckIn(c fiber.Ctx) error {
	uid, _ := ctxIDs(c)
	_, err := h.pool.Exec(context.Background(), `
		INSERT INTO user_streaks (user_id, last_active_date, current_streak, longest_streak)
		VALUES ($1, CURRENT_DATE, 1, 1)
		ON CONFLICT (user_id) DO UPDATE
		   SET current_streak = CASE
		       WHEN user_streaks.last_active_date = CURRENT_DATE THEN user_streaks.current_streak
		       WHEN user_streaks.last_active_date = CURRENT_DATE - 1 THEN user_streaks.current_streak + 1
		       ELSE 1
		   END,
		   last_active_date = CURRENT_DATE,
		   longest_streak = GREATEST(user_streaks.longest_streak,
		       CASE
		           WHEN user_streaks.last_active_date = CURRENT_DATE - 1 THEN user_streaks.current_streak + 1
		           WHEN user_streaks.last_active_date = CURRENT_DATE THEN user_streaks.current_streak
		           ELSE 1
		       END),
		   updated_at = now()
	`, uid)
	if err != nil {
		return ise(c, err)
	}
	// Auto-grant streak badges so students see them light up immediately
	// instead of waiting for a nightly worker.
	_, _ = h.pool.Exec(context.Background(), `
		INSERT INTO badge_grants (user_id, badge_id)
		SELECT $1, b.id FROM badges b JOIN user_streaks s ON s.user_id = $1
		WHERE (b.code = 'seven_day_streak' AND s.current_streak >= 7)
		   OR (b.code = 'thirty_day_streak' AND s.current_streak >= 30)
		ON CONFLICT DO NOTHING
	`, uid)
	return h.MyGamification(c)
}

func (h *Handler) Leaderboard(c fiber.Ctx) error {
	_, tid := ctxIDs(c)
	// Points per user: SUM(badge.points) for granted badges + 1 per
	// non-practice test attempt with score >= 70%.
	rows, err := h.pool.Query(context.Background(), `
		WITH badge_pts AS (
		    SELECT g.user_id, COALESCE(SUM(b.points), 0)::int AS pts
		    FROM badge_grants g JOIN badges b ON b.id = g.badge_id
		    GROUP BY g.user_id
		),
		test_pts AS (
		    SELECT user_id, COUNT(*)::int * 5 AS pts
		    FROM test_attempts
		    WHERE is_practice = FALSE AND score IS NOT NULL AND score >= 70 AND tenant_id = $1
		    GROUP BY user_id
		)
		SELECT u.id, u.full_name,
		       COALESCE(bp.pts, 0) + COALESCE(tp.pts, 0) AS points,
		       COALESCE(s.current_streak, 0) AS streak
		FROM users u
		LEFT JOIN badge_pts bp ON bp.user_id = u.id
		LEFT JOIN test_pts tp ON tp.user_id = u.id
		LEFT JOIN user_streaks s ON s.user_id = u.id
		WHERE u.tenant_id = $1 AND u.role = 'student'
		ORDER BY points DESC, streak DESC
		LIMIT 100
	`, tid)
	if err != nil {
		return ise(c, err)
	}
	out, err := scanRows(rows, "id", "full_name", "points", "streak")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(out)
}

// ──────────────────────────────────────────────────────────── WISHLIST

func (h *Handler) ListWishlist(c fiber.Ctx) error {
	uid, _ := ctxIDs(c)
	rows, err := h.pool.Query(context.Background(), `
		SELECT c.id, c.title, c.description, c.thumbnail_url, c.price, w.created_at
		FROM wishlists w JOIN courses c ON c.id = w.course_id
		WHERE w.user_id = $1 ORDER BY w.created_at DESC
	`, uid)
	if err != nil {
		return ise(c, err)
	}
	out, err := scanRows(rows, "id", "title", "description", "thumbnail_url", "price", "created_at")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) AddWishlist(c fiber.Ctx) error {
	uid, _ := ctxIDs(c)
	var body struct {
		CourseID string `json:"course_id"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return bad(c, "course_id required")
	}
	cid, err := uuid.Parse(body.CourseID)
	if err != nil {
		return bad(c, "invalid course_id")
	}
	if _, err := h.pool.Exec(context.Background(),
		`INSERT INTO wishlists (user_id, course_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		uid, cid); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"added": true})
}

func (h *Handler) RemoveWishlist(c fiber.Ctx) error {
	uid, _ := ctxIDs(c)
	cid, ok := parseUUID(c, "course_id")
	if !ok {
		return nil
	}
	if _, err := h.pool.Exec(context.Background(),
		`DELETE FROM wishlists WHERE user_id = $1 AND course_id = $2`, uid, cid); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"removed": true})
}

// ──────────────────────────────────────────────────────────── GIFTS

func (h *Handler) CreateGift(c fiber.Ctx) error {
	uid, tid := ctxIDs(c)
	var body struct {
		CourseID        string `json:"course_id"`
		RecipientPhone  string `json:"recipient_phone"`
		RecipientEmail  string `json:"recipient_email"`
		Message         string `json:"message"`
		AmountPaise     int64  `json:"amount_paise"`
		RazorpayPayment string `json:"razorpay_payment_id"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return bad(c, "invalid body")
	}
	if body.RecipientPhone == "" && body.RecipientEmail == "" {
		return bad(c, "phone or email required")
	}
	var courseID any
	if body.CourseID != "" {
		if cid, err := uuid.Parse(body.CourseID); err == nil {
			courseID = cid
		}
	}
	code := randCode(4)
	row := h.pool.QueryRow(context.Background(), `
		INSERT INTO course_gifts (tenant_id, sender_id, recipient_phone, recipient_email,
		                          course_id, amount_paise, redemption_code, message, razorpay_payment_id)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, $8, NULLIF($9, ''))
		RETURNING id, redemption_code
	`, tid, uid, body.RecipientPhone, body.RecipientEmail, courseID, body.AmountPaise, code, body.Message, body.RazorpayPayment)
	var id uuid.UUID
	var ret string
	if err := row.Scan(&id, &ret); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"id": id, "code": ret})
}

func (h *Handler) ListMyGifts(c fiber.Ctx) error {
	uid, _ := ctxIDs(c)
	rows, err := h.pool.Query(context.Background(), `
		SELECT g.id, g.recipient_phone, g.recipient_email, g.redemption_code, g.redeemed_at,
		       g.message, g.amount_paise, g.created_at, c.title
		FROM course_gifts g LEFT JOIN courses c ON c.id = g.course_id
		WHERE g.sender_id = $1 ORDER BY g.created_at DESC
	`, uid)
	if err != nil {
		return ise(c, err)
	}
	out, err := scanRows(rows, "id", "recipient_phone", "recipient_email", "redemption_code",
		"redeemed_at", "message", "amount_paise", "created_at", "course_title")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) RedeemGift(c fiber.Ctx) error {
	uid, _ := ctxIDs(c)
	var body struct {
		Code string `json:"code"`
	}
	if err := c.Bind().Body(&body); err != nil || body.Code == "" {
		return bad(c, "code required")
	}
	row := h.pool.QueryRow(context.Background(), `
		SELECT id, course_id FROM course_gifts
		WHERE redemption_code = $1 AND redeemed_at IS NULL
	`, strings.ToUpper(strings.TrimSpace(body.Code)))
	var giftID uuid.UUID
	var courseID *uuid.UUID
	if err := row.Scan(&giftID, &courseID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invalid or already-redeemed code"})
	}
	tx, err := h.pool.Begin(context.Background())
	if err != nil {
		return ise(c, err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(context.Background(),
		`UPDATE course_gifts SET redeemed_by = $1, redeemed_at = now() WHERE id = $2`,
		uid, giftID); err != nil {
		return ise(c, err)
	}
	if courseID != nil {
		// Best-effort enrolment — the existing /enrollments route does
		// the heavy lifting (RLS, idempotency); we mirror its insert
		// here so a redeemed gift doesn't sit waiting for a separate
		// enrol click.
		_, _ = tx.Exec(context.Background(),
			`INSERT INTO enrollments (course_id, user_id, status)
			 VALUES ($1, $2, 'active') ON CONFLICT DO NOTHING`,
			*courseID, uid)
	}
	if err := tx.Commit(context.Background()); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"redeemed": true, "course_id": courseID})
}

// ──────────────────────────────────────────────────────────── LECTURE NOTES

func (h *Handler) ListNotes(c fiber.Ctx) error {
	uid, _ := ctxIDs(c)
	lid, ok := parseUUID(c, "lecture_id")
	if !ok {
		return nil
	}
	rows, err := h.pool.Query(context.Background(), `
		SELECT id, timestamp_seconds, body, created_at
		FROM lecture_notes
		WHERE user_id = $1 AND lecture_id = $2
		ORDER BY timestamp_seconds ASC
	`, uid, lid)
	if err != nil {
		return ise(c, err)
	}
	out, err := scanRows(rows, "id", "timestamp_seconds", "body", "created_at")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) AddNote(c fiber.Ctx) error {
	uid, _ := ctxIDs(c)
	lid, ok := parseUUID(c, "lecture_id")
	if !ok {
		return nil
	}
	var body struct {
		TimestampSeconds int    `json:"timestamp_seconds"`
		Body             string `json:"body"`
	}
	if err := c.Bind().Body(&body); err != nil || strings.TrimSpace(body.Body) == "" {
		return bad(c, "body required")
	}
	if _, err := h.pool.Exec(context.Background(), `
		INSERT INTO lecture_notes (user_id, lecture_id, timestamp_seconds, body)
		VALUES ($1, $2, $3, $4)
	`, uid, lid, body.TimestampSeconds, body.Body); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"added": true})
}

func (h *Handler) DeleteNote(c fiber.Ctx) error {
	uid, _ := ctxIDs(c)
	id, ok := parseUUID(c, "id")
	if !ok {
		return nil
	}
	if _, err := h.pool.Exec(context.Background(),
		`DELETE FROM lecture_notes WHERE id = $1 AND user_id = $2`, id, uid); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"deleted": true})
}

// ──────────────────────────────────────────────────────────── COURSE CHAT

func (h *Handler) ListCourseChat(c fiber.Ctx) error {
	cid, ok := parseUUID(c, "course_id")
	if !ok {
		return nil
	}
	rows, err := h.pool.Query(context.Background(), `
		SELECT m.id, m.body, m.created_at, u.full_name, m.user_id
		FROM course_chat_messages m
		JOIN users u ON u.id = m.user_id
		WHERE m.course_id = $1
		ORDER BY m.created_at DESC
		LIMIT 200
	`, cid)
	if err != nil {
		return ise(c, err)
	}
	out, err := scanRows(rows, "id", "body", "created_at", "full_name", "user_id")
	if err != nil {
		return ise(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) SendCourseChat(c fiber.Ctx) error {
	uid, tid := ctxIDs(c)
	cid, ok := parseUUID(c, "course_id")
	if !ok {
		return nil
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := c.Bind().Body(&body); err != nil || strings.TrimSpace(body.Body) == "" {
		return bad(c, "body required")
	}
	if _, err := h.pool.Exec(context.Background(), `
		INSERT INTO course_chat_messages (tenant_id, course_id, user_id, body)
		VALUES ($1, $2, $3, $4)
	`, tid, cid, uid, body.Body); err != nil {
		return ise(c, err)
	}
	return c.JSON(fiber.Map{"sent": true})
}
