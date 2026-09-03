package grpcserver

import (
	"context"
	"errors"

	engagementv1 "live-platform/gen/proto/live/engagement/v1"
	"live-platform/internal/engagement"
	"live-platform/internal/utils"
)

type EngagementServer struct {
	engagementv1.UnimplementedEngagementServiceServer
	svc *engagement.Service
}

func NewEngagementServer(svc *engagement.Service) *EngagementServer {
	return &EngagementServer{svc: svc}
}

func engIsStaff(c caller) bool {
	switch c.Role {
	case "instructor", "admin", "owner", "super_admin":
		return true
	}
	return false
}

func (s *EngagementServer) ListReviews(ctx context.Context, req *engagementv1.ListReviewsRequest) (*engagementv1.ListReviewsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListReviews(ctx, c.TenantID, courseID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &engagementv1.ListReviewsResponse{}
	for _, r := range rows {
		out.Reviews = append(out.Reviews, &engagementv1.Review{
			Id: utils.UUIDFromPg(r.ID), Rating: int32(r.Rating), Body: r.Body,
			FullName: utils.TextFromPg(r.FullName), CreatedAt: tsFromPgtz(r.CreatedAt),
		})
	}
	return out, nil
}

func (s *EngagementServer) GetReviewSummary(ctx context.Context, req *engagementv1.GetReviewSummaryRequest) (*engagementv1.GetReviewSummaryResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	row, err := s.svc.ReviewSummary(ctx, c.TenantID, courseID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.GetReviewSummaryResponse{Average: utils.NumericToFloat(row.AvgRating), Count: row.Total}, nil
}

func (s *EngagementServer) UpsertReview(ctx context.Context, req *engagementv1.UpsertReviewRequest) (*engagementv1.UpsertReviewResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesStudentUp...); err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	if req.GetRating() < 1 || req.GetRating() > 5 {
		return nil, invalidArg("rating must be 1..5")
	}
	r, err := s.svc.UpsertReview(ctx, c.TenantID, c.UserID, courseID, int(req.GetRating()), req.GetBody())
	if err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.UpsertReviewResponse{Review: &engagementv1.Review{
		Id: utils.UUIDFromPg(r.ID), Rating: int32(r.Rating), Body: r.Body, IsApproved: r.IsApproved, CreatedAt: tsFromPgtz(r.CreatedAt),
	}}, nil
}

func (s *EngagementServer) AdminListReviews(ctx context.Context, req *engagementv1.AdminListReviewsRequest) (*engagementv1.AdminListReviewsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.AdminListReviews(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &engagementv1.AdminListReviewsResponse{}
	for _, r := range rows {
		out.Reviews = append(out.Reviews, &engagementv1.Review{
			Id: utils.UUIDFromPg(r.ID), Rating: int32(r.Rating), Body: r.Body, IsApproved: r.IsApproved,
			FullName: utils.TextFromPg(r.FullName), CreatedAt: tsFromPgtz(r.CreatedAt),
		})
	}
	return out, nil
}

func (s *EngagementServer) SetReviewApproved(ctx context.Context, req *engagementv1.SetReviewApprovedRequest) (*engagementv1.SetReviewApprovedResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetReviewId(), "review_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetReviewApproved(ctx, id, req.GetApproved()); err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.SetReviewApprovedResponse{}, nil
}

func (s *EngagementServer) DeleteReview(ctx context.Context, req *engagementv1.DeleteReviewRequest) (*engagementv1.DeleteReviewResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetReviewId(), "review_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeleteReview(ctx, c.TenantID, id); err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.DeleteReviewResponse{}, nil
}

func (s *EngagementServer) ListThreads(ctx context.Context, req *engagementv1.ListThreadsRequest) (*engagementv1.ListThreadsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := optUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListThreads(ctx, c.TenantID, courseID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &engagementv1.ListThreadsResponse{}
	for _, t := range rows {
		out.Threads = append(out.Threads, &engagementv1.Thread{
			Id: utils.UUIDFromPg(t.ID), Title: t.Title, IsPinned: t.IsPinned, IsLocked: t.IsLocked,
			ReplyCount: t.ReplyCount, FullName: utils.TextFromPg(t.FullName),
			LastReplyAt: tsFromPgtz(t.LastReplyAt), CreatedAt: tsFromPgtz(t.CreatedAt),
		})
	}
	return out, nil
}

func (s *EngagementServer) CreateThread(ctx context.Context, req *engagementv1.CreateThreadRequest) (*engagementv1.CreateThreadResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesStudentUp...); err != nil {
		return nil, err
	}
	if req.GetTitle() == "" {
		return nil, invalidArg("title is required")
	}
	courseID, err := optUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.CreateThread(ctx, c.TenantID, c.UserID, courseID, req.GetTitle(), req.GetBody())
	if err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.CreateThreadResponse{Id: utils.UUIDFromPg(r.ID)}, nil
}

func (s *EngagementServer) ListPosts(ctx context.Context, req *engagementv1.ListPostsRequest) (*engagementv1.ListPostsResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	threadID, err := parseUUID(req.GetThreadId(), "thread_id")
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListPosts(ctx, threadID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &engagementv1.ListPostsResponse{}
	for _, p := range rows {
		out.Posts = append(out.Posts, &engagementv1.Post{
			Id: utils.UUIDFromPg(p.ID), Body: p.Body, IsInstructorHighlight: p.IsInstructorHighlight,
			FullName: utils.TextFromPg(p.FullName), CreatedAt: tsFromPgtz(p.CreatedAt),
		})
	}
	return out, nil
}

func (s *EngagementServer) CreatePost(ctx context.Context, req *engagementv1.CreatePostRequest) (*engagementv1.CreatePostResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesStudentUp...); err != nil {
		return nil, err
	}
	threadID, err := parseUUID(req.GetThreadId(), "thread_id")
	if err != nil {
		return nil, err
	}
	if req.GetBody() == "" {
		return nil, invalidArg("body is required")
	}
	r, err := s.svc.AddPost(ctx, c.TenantID, threadID, c.UserID, req.GetBody(), engIsStaff(c))
	if err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.CreatePostResponse{Id: utils.UUIDFromPg(r.ID), IsInstructorHighlight: r.IsInstructorHighlight}, nil
}

func (s *EngagementServer) SetThreadPinned(ctx context.Context, req *engagementv1.SetThreadPinnedRequest) (*engagementv1.SetThreadPinnedResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetThreadId(), "thread_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetThreadPinned(ctx, c.TenantID, id, req.GetPinned()); err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.SetThreadPinnedResponse{}, nil
}

func (s *EngagementServer) SetThreadLocked(ctx context.Context, req *engagementv1.SetThreadLockedRequest) (*engagementv1.SetThreadLockedResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetThreadId(), "thread_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetThreadLocked(ctx, c.TenantID, id, req.GetLocked()); err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.SetThreadLockedResponse{}, nil
}

func (s *EngagementServer) ListBadges(ctx context.Context, _ *engagementv1.ListBadgesRequest) (*engagementv1.ListBadgesResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	rows, err := s.svc.ListBadges(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &engagementv1.ListBadgesResponse{}
	for _, b := range rows {
		out.Badges = append(out.Badges, &engagementv1.Badge{
			Id: utils.UUIDFromPg(b.ID), Code: b.Code, Name: b.Name,
			Description: utils.TextFromPg(b.Description), Icon: utils.TextFromPg(b.Icon), Points: b.Points,
		})
	}
	return out, nil
}

func gamMsg(g engagement.Gamification) *engagementv1.Gamification {
	m := &engagementv1.Gamification{
		CurrentStreak: g.Streak.CurrentStreak, LongestStreak: g.Streak.LongestStreak, TotalPoints: g.Streak.TotalPoints,
	}
	for _, b := range g.Badges {
		m.Badges = append(m.Badges, &engagementv1.Badge{
			Code: b.Code, Name: b.Name, Icon: utils.TextFromPg(b.Icon), Points: b.Points, EarnedAt: tsFromPgtz(b.EarnedAt),
		})
	}
	return m
}

func (s *EngagementServer) GetMyGamification(ctx context.Context, _ *engagementv1.GetMyGamificationRequest) (*engagementv1.GetMyGamificationResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.svc.MyGamification(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.GetMyGamificationResponse{Gamification: gamMsg(g)}, nil
}

func (s *EngagementServer) CheckIn(ctx context.Context, _ *engagementv1.CheckInRequest) (*engagementv1.CheckInResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.svc.CheckIn(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.CheckInResponse{Gamification: gamMsg(g)}, nil
}

func (s *EngagementServer) Leaderboard(ctx context.Context, req *engagementv1.LeaderboardRequest) (*engagementv1.LeaderboardResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.Leaderboard(ctx, c.TenantID, req.GetLimit())
	if err != nil {
		return nil, toStatus(err)
	}
	out := &engagementv1.LeaderboardResponse{}
	for _, r := range rows {
		out.Rows = append(out.Rows, &engagementv1.LeaderRow{
			UserId: utils.UUIDFromPg(r.UserID), FullName: utils.TextFromPg(r.FullName),
			Points: r.TotalPoints, Streak: r.CurrentStreak,
		})
	}
	return out, nil
}

func (s *EngagementServer) ListWishlist(ctx context.Context, _ *engagementv1.ListWishlistRequest) (*engagementv1.ListWishlistResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListWishlist(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &engagementv1.ListWishlistResponse{}
	for _, w := range rows {
		out.Items = append(out.Items, &engagementv1.WishlistItem{
			CourseId: utils.UUIDFromPg(w.CourseID), Title: w.Title, Slug: w.Slug, ThumbnailUrl: utils.TextFromPg(w.ThumbnailUrl),
		})
	}
	return out, nil
}

func (s *EngagementServer) AddWishlist(ctx context.Context, req *engagementv1.AddWishlistRequest) (*engagementv1.AddWishlistResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.AddWishlist(ctx, c.TenantID, c.UserID, courseID); err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.AddWishlistResponse{}, nil
}

func (s *EngagementServer) RemoveWishlist(ctx context.Context, req *engagementv1.RemoveWishlistRequest) (*engagementv1.RemoveWishlistResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.RemoveWishlist(ctx, c.TenantID, c.UserID, courseID); err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.RemoveWishlistResponse{}, nil
}

func (s *EngagementServer) CreateGift(ctx context.Context, req *engagementv1.CreateGiftRequest) (*engagementv1.CreateGiftResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetRecipientPhone() == "" && req.GetRecipientEmail() == "" {
		return nil, invalidArg("recipient_phone or recipient_email is required")
	}
	pid, err := optUUID(req.GetProductId(), "product_id")
	if err != nil {
		return nil, err
	}
	oid, err := optUUID(req.GetOrderId(), "order_id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.CreateGift(ctx, c.TenantID, c.UserID, engagement.CreateGiftInput{
		ProductID: pid, OrderID: oid, RecipientPhone: req.GetRecipientPhone(),
		RecipientEmail: req.GetRecipientEmail(), Message: req.GetMessage(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.CreateGiftResponse{Id: utils.UUIDFromPg(r.ID), Code: r.RedemptionCode}, nil
}

func (s *EngagementServer) ListMyGifts(ctx context.Context, _ *engagementv1.ListMyGiftsRequest) (*engagementv1.ListMyGiftsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListMyGifts(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &engagementv1.ListMyGiftsResponse{}
	for _, g := range rows {
		out.Gifts = append(out.Gifts, &engagementv1.Gift{
			Id: utils.UUIDFromPg(g.ID), RecipientPhone: utils.TextFromPg(g.RecipientPhone),
			RecipientEmail: utils.TextFromPg(g.RecipientEmail), RedemptionCode: g.RedemptionCode,
			CourseTitle: utils.TextFromPg(g.CourseTitle), RedeemedAt: tsFromPgtz(g.RedeemedAt),
		})
	}
	return out, nil
}

func (s *EngagementServer) RedeemGift(ctx context.Context, req *engagementv1.RedeemGiftRequest) (*engagementv1.RedeemGiftResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	r, err := s.svc.RedeemGift(ctx, c.UserID, req.GetCode())
	if errors.Is(err, engagement.ErrGiftNotRedeemable) {
		return nil, invalidArg(err.Error())
	}
	if err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.RedeemGiftResponse{ProductId: utils.UUIDFromPg(r.ProductID)}, nil
}

func (s *EngagementServer) ListCourseChat(ctx context.Context, req *engagementv1.ListCourseChatRequest) (*engagementv1.ListCourseChatResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListCourseChat(ctx, c.TenantID, courseID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &engagementv1.ListCourseChatResponse{}
	for _, m := range rows {
		out.Messages = append(out.Messages, &engagementv1.ChatMessage{
			Id: utils.UUIDFromPg(m.ID), Body: m.Body, FullName: utils.TextFromPg(m.FullName), CreatedAt: tsFromPgtz(m.CreatedAt),
		})
	}
	return out, nil
}

func (s *EngagementServer) SendCourseChat(ctx context.Context, req *engagementv1.SendCourseChatRequest) (*engagementv1.SendCourseChatResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	if req.GetBody() == "" {
		return nil, invalidArg("body is required")
	}
	r, err := s.svc.SendCourseChat(ctx, c.TenantID, courseID, c.UserID, req.GetBody())
	if err != nil {
		return nil, toStatus(err)
	}
	return &engagementv1.SendCourseChatResponse{Id: utils.UUIDFromPg(r.ID)}, nil
}
