package engagement

import (
	"errors"

	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

// ─────────────────────────────────────────────────────────────── helpers

func badReq(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": msg})
}

func serverErr(c fiber.Ctx, err error) error {
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
}

func paramUUID(c fiber.Ctx, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Params(key))
	return id, err == nil
}

func optUUID(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

func isStaff(c fiber.Ctx) bool {
	switch c.Locals(middleware.LocalRole) {
	case "instructor", "admin", "owner", "super_admin":
		return true
	}
	return false
}

func qInt(c fiber.Ctx, key string, def int32) int32 {
	return int32(fiber.Query[int](c, key, int(def)))
}

// ─────────────────────────────────────────────────────────────── reviews

// ListReviews — GET /engagement/courses/:id/reviews
func (h *Handler) ListReviews(c fiber.Ctx) error {
	courseID, ok := paramUUID(c, "id")
	if !ok {
		return badReq(c, "invalid course id")
	}
	rows, err := h.svc.ListReviews(c.Context(), middleware.CurrentTenantID(c), courseID, qInt(c, "limit", 50), qInt(c, "offset", 0))
	if err != nil {
		return serverErr(c, err)
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "rating": r.Rating, "body": r.Body,
			"created_at": r.CreatedAt.Time, "full_name": utils.TextFromPg(r.FullName),
			"avatar_url": utils.TextFromPg(r.AvatarUrl),
		}
	}
	return c.JSON(out)
}

// ReviewSummary — GET /engagement/courses/:id/review-summary
func (h *Handler) ReviewSummary(c fiber.Ctx) error {
	courseID, ok := paramUUID(c, "id")
	if !ok {
		return badReq(c, "invalid course id")
	}
	row, err := h.svc.ReviewSummary(c.Context(), middleware.CurrentTenantID(c), courseID)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(fiber.Map{"average": utils.NumericToFloat(row.AvgRating), "count": row.Total})
}

// UpsertReview — POST /engagement/courses/:id/reviews
func (h *Handler) UpsertReview(c fiber.Ctx) error {
	courseID, ok := paramUUID(c, "id")
	if !ok {
		return badReq(c, "invalid course id")
	}
	var body struct {
		Rating int    `json:"rating"`
		Body   string `json:"body"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.Rating < 1 || body.Rating > 5 {
		return badReq(c, "rating must be 1..5")
	}
	row, err := h.svc.UpsertReview(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), courseID, body.Rating, body.Body)
	if err != nil {
		return badReq(c, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": utils.UUIDFromPg(row.ID), "rating": row.Rating, "body": row.Body,
		"is_approved": row.IsApproved, "created_at": row.CreatedAt.Time,
	})
}

// AdminListReviews — GET /admin/engagement/reviews
func (h *Handler) AdminListReviews(c fiber.Ctx) error {
	rows, err := h.svc.AdminListReviews(c.Context(), middleware.CurrentTenantID(c), qInt(c, "limit", 100), qInt(c, "offset", 0))
	if err != nil {
		return serverErr(c, err)
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(r.ID), "rating": r.Rating, "body": r.Body,
			"is_approved": r.IsApproved, "created_at": r.CreatedAt.Time,
			"full_name": utils.TextFromPg(r.FullName), "course_title": r.CourseTitle,
		}
	}
	return c.JSON(out)
}

// SetReviewApproved — POST /admin/engagement/reviews/:id/approve
func (h *Handler) SetReviewApproved(c fiber.Ctx) error {
	id, ok := paramUUID(c, "id")
	if !ok {
		return badReq(c, "invalid id")
	}
	var body struct {
		Approved bool `json:"approved"`
	}
	_ = c.Bind().JSON(&body)
	if err := h.svc.SetReviewApproved(c.Context(), id, body.Approved); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(fiber.Map{"updated": true})
}

// DeleteReview — DELETE /admin/engagement/reviews/:id
func (h *Handler) DeleteReview(c fiber.Ctx) error {
	id, ok := paramUUID(c, "id")
	if !ok {
		return badReq(c, "invalid id")
	}
	if err := h.svc.DeleteReview(c.Context(), middleware.CurrentTenantID(c), id); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(fiber.Map{"deleted": true})
}

// ─────────────────────────────────────────────────────────────── forum

// ListThreads — GET /engagement/forum/threads?course_id=
func (h *Handler) ListThreads(c fiber.Ctx) error {
	rows, err := h.svc.ListThreads(c.Context(), middleware.CurrentTenantID(c), optUUID(c.Query("course_id")), qInt(c, "limit", 50), qInt(c, "offset", 0))
	if err != nil {
		return serverErr(c, err)
	}
	out := make([]fiber.Map, len(rows))
	for i, t := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(t.ID), "title": t.Title, "is_pinned": t.IsPinned,
			"is_locked": t.IsLocked, "reply_count": t.ReplyCount,
			"last_reply_at": t.LastReplyAt.Time, "created_at": t.CreatedAt.Time,
			"full_name": utils.TextFromPg(t.FullName),
		}
	}
	return c.JSON(out)
}

// CreateThread — POST /engagement/forum/threads
func (h *Handler) CreateThread(c fiber.Ctx) error {
	var body struct {
		CourseID string `json:"course_id"`
		Title    string `json:"title"`
		Body     string `json:"body"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.Title == "" {
		return badReq(c, "title required")
	}
	row, err := h.svc.CreateThread(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), optUUID(body.CourseID), body.Title, body.Body)
	if err != nil {
		return badReq(c, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": utils.UUIDFromPg(row.ID), "created_at": row.CreatedAt.Time})
}

// ListPosts — GET /engagement/forum/threads/:id/posts
func (h *Handler) ListPosts(c fiber.Ctx) error {
	threadID, ok := paramUUID(c, "id")
	if !ok {
		return badReq(c, "invalid thread id")
	}
	rows, err := h.svc.ListPosts(c.Context(), threadID, qInt(c, "limit", 100), qInt(c, "offset", 0))
	if err != nil {
		return serverErr(c, err)
	}
	out := make([]fiber.Map, len(rows))
	for i, p := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(p.ID), "body": p.Body,
			"is_instructor_highlight": p.IsInstructorHighlight, "created_at": p.CreatedAt.Time,
			"full_name": utils.TextFromPg(p.FullName), "avatar_url": utils.TextFromPg(p.AvatarUrl),
		}
	}
	return c.JSON(out)
}

// CreatePost — POST /engagement/forum/threads/:id/posts
func (h *Handler) CreatePost(c fiber.Ctx) error {
	threadID, ok := paramUUID(c, "id")
	if !ok {
		return badReq(c, "invalid thread id")
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.Body == "" {
		return badReq(c, "body required")
	}
	row, err := h.svc.AddPost(c.Context(), middleware.CurrentTenantID(c), threadID, middleware.CurrentUserID(c), body.Body, isStaff(c))
	if err != nil {
		return serverErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": utils.UUIDFromPg(row.ID), "is_instructor_highlight": row.IsInstructorHighlight,
		"created_at": row.CreatedAt.Time,
	})
}

// SetThreadPinned — POST /admin/engagement/forum/threads/:id/pin
func (h *Handler) SetThreadPinned(c fiber.Ctx) error {
	id, ok := paramUUID(c, "id")
	if !ok {
		return badReq(c, "invalid id")
	}
	var body struct {
		Pinned bool `json:"pinned"`
	}
	_ = c.Bind().JSON(&body)
	if err := h.svc.SetThreadPinned(c.Context(), middleware.CurrentTenantID(c), id, body.Pinned); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(fiber.Map{"updated": true})
}

// SetThreadLocked — POST /admin/engagement/forum/threads/:id/lock
func (h *Handler) SetThreadLocked(c fiber.Ctx) error {
	id, ok := paramUUID(c, "id")
	if !ok {
		return badReq(c, "invalid id")
	}
	var body struct {
		Locked bool `json:"locked"`
	}
	_ = c.Bind().JSON(&body)
	if err := h.svc.SetThreadLocked(c.Context(), middleware.CurrentTenantID(c), id, body.Locked); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(fiber.Map{"updated": true})
}

// ─────────────────────────────────────────────────────────── gamification

// ListBadges — GET /engagement/badges
func (h *Handler) ListBadges(c fiber.Ctx) error {
	rows, err := h.svc.ListBadges(c.Context())
	if err != nil {
		return serverErr(c, err)
	}
	out := make([]fiber.Map, len(rows))
	for i, b := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(b.ID), "code": b.Code, "name": b.Name,
			"description": utils.TextFromPg(b.Description), "icon": utils.TextFromPg(b.Icon), "points": b.Points,
		}
	}
	return c.JSON(out)
}

func gamificationJSON(g Gamification) fiber.Map {
	badges := make([]fiber.Map, len(g.Badges))
	for i, b := range g.Badges {
		badges[i] = fiber.Map{
			"code": b.Code, "name": b.Name, "icon": utils.TextFromPg(b.Icon),
			"points": b.Points, "earned_at": b.EarnedAt.Time,
		}
	}
	return fiber.Map{
		"current_streak": g.Streak.CurrentStreak, "longest_streak": g.Streak.LongestStreak,
		"total_points": g.Streak.TotalPoints, "last_active_date": g.Streak.LastActiveDate.Time,
		"badges": badges,
	}
}

// MyGamification — GET /engagement/gamification/me
func (h *Handler) MyGamification(c fiber.Ctx) error {
	g, err := h.svc.MyGamification(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c))
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(gamificationJSON(g))
}

// CheckIn — POST /engagement/gamification/check-in
func (h *Handler) CheckIn(c fiber.Ctx) error {
	g, err := h.svc.CheckIn(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c))
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(gamificationJSON(g))
}

// Leaderboard — GET /engagement/gamification/leaderboard
func (h *Handler) Leaderboard(c fiber.Ctx) error {
	rows, err := h.svc.Leaderboard(c.Context(), middleware.CurrentTenantID(c), qInt(c, "limit", 50))
	if err != nil {
		return serverErr(c, err)
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"user_id": utils.UUIDFromPg(r.UserID), "full_name": utils.TextFromPg(r.FullName),
			"avatar_url": utils.TextFromPg(r.AvatarUrl), "points": r.TotalPoints, "streak": r.CurrentStreak,
		}
	}
	return c.JSON(out)
}

// ─────────────────────────────────────────────────────────────── wishlist

// ListWishlist — GET /engagement/wishlist
func (h *Handler) ListWishlist(c fiber.Ctx) error {
	rows, err := h.svc.ListWishlist(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c))
	if err != nil {
		return serverErr(c, err)
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"course_id": utils.UUIDFromPg(r.CourseID), "title": r.Title, "slug": r.Slug,
			"thumbnail_url": utils.TextFromPg(r.ThumbnailUrl), "created_at": r.CreatedAt.Time,
		}
	}
	return c.JSON(out)
}

// AddWishlist — POST /engagement/wishlist
func (h *Handler) AddWishlist(c fiber.Ctx) error {
	var body struct {
		CourseID string `json:"course_id"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return badReq(c, "course_id required")
	}
	cid, err := uuid.Parse(body.CourseID)
	if err != nil {
		return badReq(c, "invalid course_id")
	}
	if err := h.svc.AddWishlist(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), cid); err != nil {
		return serverErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"added": true})
}

// RemoveWishlist — DELETE /engagement/wishlist/:course_id
func (h *Handler) RemoveWishlist(c fiber.Ctx) error {
	cid, ok := paramUUID(c, "course_id")
	if !ok {
		return badReq(c, "invalid course_id")
	}
	if err := h.svc.RemoveWishlist(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), cid); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(fiber.Map{"removed": true})
}

// ─────────────────────────────────────────────────────────────── gifts

// CreateGift — POST /engagement/gifts
func (h *Handler) CreateGift(c fiber.Ctx) error {
	var body struct {
		ProductID      string `json:"product_id"`
		OrderID        string `json:"order_id"`
		RecipientPhone string `json:"recipient_phone"`
		RecipientEmail string `json:"recipient_email"`
		Message        string `json:"message"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return badReq(c, "invalid body")
	}
	if body.RecipientPhone == "" && body.RecipientEmail == "" {
		return badReq(c, "recipient_phone or recipient_email required")
	}
	row, err := h.svc.CreateGift(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), CreateGiftInput{
		ProductID: optUUID(body.ProductID), OrderID: optUUID(body.OrderID),
		RecipientPhone: body.RecipientPhone, RecipientEmail: body.RecipientEmail, Message: body.Message,
	})
	if err != nil {
		return badReq(c, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": utils.UUIDFromPg(row.ID), "code": row.RedemptionCode, "created_at": row.CreatedAt.Time,
	})
}

// ListMyGifts — GET /engagement/gifts/mine
func (h *Handler) ListMyGifts(c fiber.Ctx) error {
	rows, err := h.svc.ListMyGifts(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c))
	if err != nil {
		return serverErr(c, err)
	}
	out := make([]fiber.Map, len(rows))
	for i, g := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(g.ID), "recipient_phone": utils.TextFromPg(g.RecipientPhone),
			"recipient_email": utils.TextFromPg(g.RecipientEmail), "redemption_code": g.RedemptionCode,
			"redeemed_at": g.RedeemedAt.Time, "message": utils.TextFromPg(g.Message),
			"created_at": g.CreatedAt.Time, "course_title": utils.TextFromPg(g.CourseTitle),
		}
	}
	return c.JSON(out)
}

// RedeemGift — POST /engagement/gifts/redeem
func (h *Handler) RedeemGift(c fiber.Ctx) error {
	var body struct {
		Code string `json:"code"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.Code == "" {
		return badReq(c, "code required")
	}
	row, err := h.svc.RedeemGift(c.Context(), middleware.CurrentUserID(c), body.Code)
	if errors.Is(err, ErrGiftNotRedeemable) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(fiber.Map{"redeemed": true, "product_id": utils.UUIDFromPg(row.ProductID)})
}

// ─────────────────────────────────────────────────────────── course chat

// ListCourseChat — GET /engagement/courses/:course_id/chat
func (h *Handler) ListCourseChat(c fiber.Ctx) error {
	cid, ok := paramUUID(c, "course_id")
	if !ok {
		return badReq(c, "invalid course_id")
	}
	rows, err := h.svc.ListCourseChat(c.Context(), middleware.CurrentTenantID(c), cid, qInt(c, "limit", 100), qInt(c, "offset", 0))
	if err != nil {
		return serverErr(c, err)
	}
	out := make([]fiber.Map, len(rows))
	for i, m := range rows {
		out[i] = fiber.Map{
			"id": utils.UUIDFromPg(m.ID), "body": m.Body, "created_at": m.CreatedAt.Time,
			"full_name": utils.TextFromPg(m.FullName), "avatar_url": utils.TextFromPg(m.AvatarUrl),
		}
	}
	return c.JSON(out)
}

// SendCourseChat — POST /engagement/courses/:course_id/chat
func (h *Handler) SendCourseChat(c fiber.Ctx) error {
	cid, ok := paramUUID(c, "course_id")
	if !ok {
		return badReq(c, "invalid course_id")
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.Body == "" {
		return badReq(c, "body required")
	}
	row, err := h.svc.SendCourseChat(c.Context(), middleware.CurrentTenantID(c), cid, middleware.CurrentUserID(c), body.Body)
	if err != nil {
		return serverErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": utils.UUIDFromPg(row.ID), "created_at": row.CreatedAt.Time})
}
