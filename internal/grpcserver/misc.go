package grpcserver

import (
	"context"

	bannersv1 "live-platform/gen/proto/live/banners/v1"
	leadsv1 "live-platform/gen/proto/live/leads/v1"
	notificationsv1 "live-platform/gen/proto/live/notifications/v1"
	referralsv1 "live-platform/gen/proto/live/referrals/v1"
	schedulev1 "live-platform/gen/proto/live/schedule/v1"
	searchv1 "live-platform/gen/proto/live/search/v1"
	"live-platform/internal/banners"
	"live-platform/internal/leads"
	"live-platform/internal/notifications"
	"live-platform/internal/referrals"
	"live-platform/internal/schedule"
	"live-platform/internal/search"
	"live-platform/internal/utils"

	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────── search

type SearchServer struct {
	searchv1.UnimplementedSearchServiceServer
	svc *search.Service
}

func NewSearchServer(svc *search.Service) *SearchServer { return &SearchServer{svc: svc} }

func (s *SearchServer) Search(ctx context.Context, req *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.Unified(ctx, c.TenantID, req.GetQuery(), limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &searchv1.SearchResponse{}
	for _, r := range rows {
		out.Results = append(out.Results, &searchv1.SearchResult{
			Type: r.Type, Id: r.ID, Title: r.Title, Description: r.Description,
			Snippet: r.Snippet, ThumbnailUrl: r.Thumbnail,
		})
	}
	return out, nil
}

// ────────────────────────────────────────────────────────────── referrals

type ReferralServer struct {
	referralsv1.UnimplementedReferralServiceServer
	svc *referrals.Service
}

func NewReferralServer(svc *referrals.Service) *ReferralServer { return &ReferralServer{svc: svc} }

func (s *ReferralServer) GetMyReferral(ctx context.Context, _ *referralsv1.GetMyReferralRequest) (*referralsv1.GetMyReferralResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	r, err := s.svc.MyCode(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &referralsv1.GetMyReferralResponse{
		Code: r.Code, Uses: int32(r.Uses), PendingCount: r.PendingCount,
		RewardedCount: r.RewardedCount, TotalRewardedMinor: r.TotalRewardedPaise,
	}, nil
}

// ────────────────────────────────────────────────────────── notifications

type NotificationServer struct {
	notificationsv1.UnimplementedNotificationServiceServer
	svc *notifications.Service
}

func NewNotificationServer(svc *notifications.Service) *NotificationServer {
	return &NotificationServer{svc: svc}
}

func (s *NotificationServer) ListMyNotifications(ctx context.Context, req *notificationsv1.ListMyNotificationsRequest) (*notificationsv1.ListMyNotificationsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListMine(ctx, c.TenantID, c.UserID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &notificationsv1.ListMyNotificationsResponse{}
	for _, n := range rows {
		out.Notifications = append(out.Notifications, &notificationsv1.Notification{
			Id: utils.UUIDFromPg(n.ID), TemplateKey: n.TemplateKey, Title: n.Title, Body: utils.TextFromPg(n.Body),
			EntityType: utils.TextFromPg(n.EntityType), EntityId: utils.UUIDFromPg(n.EntityID),
			ReadAt: tsFromPgtz(n.ReadAt), CreatedAt: tsFromPgtz(n.CreatedAt),
		})
	}
	return out, nil
}

func (s *NotificationServer) UnreadCount(ctx context.Context, _ *notificationsv1.UnreadCountRequest) (*notificationsv1.UnreadCountResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.svc.UnreadCount(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &notificationsv1.UnreadCountResponse{Count: n}, nil
}

func (s *NotificationServer) MarkRead(ctx context.Context, req *notificationsv1.MarkReadRequest) (*notificationsv1.MarkReadResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.MarkRead(ctx, id, c.UserID); err != nil {
		return nil, toStatus(err)
	}
	return &notificationsv1.MarkReadResponse{}, nil
}

func (s *NotificationServer) MarkAllRead(ctx context.Context, _ *notificationsv1.MarkAllReadRequest) (*notificationsv1.MarkAllReadResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.svc.MarkAllRead(ctx, c.TenantID, c.UserID); err != nil {
		return nil, toStatus(err)
	}
	return &notificationsv1.MarkAllReadResponse{}, nil
}

func (s *NotificationServer) DeleteNotification(ctx context.Context, req *notificationsv1.DeleteNotificationRequest) (*notificationsv1.DeleteNotificationResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, id, c.UserID); err != nil {
		return nil, toStatus(err)
	}
	return &notificationsv1.DeleteNotificationResponse{}, nil
}

func (s *NotificationServer) CreateAnnouncement(ctx context.Context, req *notificationsv1.CreateAnnouncementRequest) (*notificationsv1.CreateAnnouncementResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	courseID, err := optUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	batchID, err := optUUID(req.GetBatchId(), "batch_id")
	if err != nil {
		return nil, err
	}
	in := notifications.CreateAnnouncementRequest{
		CourseID: courseID, BatchID: batchID, Title: req.GetTitle(), Body: req.GetBody(),
		Priority: req.GetPriority(), FanOut: req.GetFanOut(),
	}
	if req.GetExpiresAt() != nil {
		t := req.GetExpiresAt().AsTime()
		in.ExpiresAt = &t
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	a, err := s.svc.CreateAnnouncement(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &notificationsv1.CreateAnnouncementResponse{Announcement: &notificationsv1.Announcement{
		Id: utils.UUIDFromPg(a.ID), Title: a.Title, Body: a.Body, Priority: a.Priority, PublishedAt: tsFromPgtz(a.PublishedAt),
	}}, nil
}

func (s *NotificationServer) ListAnnouncements(ctx context.Context, req *notificationsv1.ListAnnouncementsRequest) (*notificationsv1.ListAnnouncementsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := optUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	batchID, err := optUUID(req.GetBatchId(), "batch_id")
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListAnnouncements(ctx, c.TenantID, courseID, batchID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &notificationsv1.ListAnnouncementsResponse{}
	for _, a := range rows {
		out.Announcements = append(out.Announcements, &notificationsv1.Announcement{
			Id: utils.UUIDFromPg(a.ID), CourseId: utils.UUIDFromPg(a.CourseID), BatchId: utils.UUIDFromPg(a.BatchID),
			Title: a.Title, Body: a.Body, Priority: a.Priority,
			PublishedAt: tsFromPgtz(a.PublishedAt), ExpiresAt: tsFromPgtz(a.ExpiresAt),
		})
	}
	return out, nil
}

func (s *NotificationServer) DeleteAnnouncement(ctx context.Context, req *notificationsv1.DeleteAnnouncementRequest) (*notificationsv1.DeleteAnnouncementResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeleteAnnouncement(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &notificationsv1.DeleteAnnouncementResponse{}, nil
}

// ─────────────────────────────────────────────────────────────── banners

type BannerServer struct {
	bannersv1.UnimplementedBannerServiceServer
	svc *banners.Service
}

func NewBannerServer(svc *banners.Service) *BannerServer { return &BannerServer{svc: svc} }

func bannerMsg(b banners.Banner) *bannersv1.Banner {
	return &bannersv1.Banner{
		Id: b.ID, Title: b.Title, Subtitle: b.Subtitle, ImageUrl: b.ImageURL, BackgroundColor: b.BackgroundColor,
		LinkType: b.LinkType, LinkId: b.LinkID, LinkUrl: b.LinkURL, DisplayOrder: b.DisplayOrder,
		IsActive: b.IsActive, StartsAt: tsFromTime(b.StartsAt), EndsAt: tsFromTime(b.EndsAt),
	}
}

func bannerInput(in *bannersv1.BannerInput) (banners.UpsertBannerRequest, error) {
	r := banners.UpsertBannerRequest{
		Title: in.GetTitle(), Subtitle: in.GetSubtitle(), ImageURL: in.GetImageUrl(),
		BackgroundColor: in.GetBackgroundColor(), LinkType: in.GetLinkType(), LinkURL: in.GetLinkUrl(),
		DisplayOrder: in.GetDisplayOrder(), IsActive: in.GetIsActive(),
	}
	lid, err := optUUID(in.GetLinkId(), "link_id")
	if err != nil {
		return r, err
	}
	r.LinkID = lid
	if in.GetStartsAt() != nil {
		t := in.GetStartsAt().AsTime()
		r.StartsAt = &t
	}
	if in.GetEndsAt() != nil {
		t := in.GetEndsAt().AsTime()
		r.EndsAt = &t
	}
	return r, nil
}

func (s *BannerServer) ListActiveBanners(ctx context.Context, _ *bannersv1.ListActiveBannersRequest) (*bannersv1.ListActiveBannersResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListActive(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &bannersv1.ListActiveBannersResponse{}
	for _, b := range rows {
		out.Banners = append(out.Banners, bannerMsg(b))
	}
	return out, nil
}

func (s *BannerServer) ListAllBanners(ctx context.Context, req *bannersv1.ListAllBannersRequest) (*bannersv1.ListAllBannersResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListAll(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &bannersv1.ListAllBannersResponse{}
	for _, b := range rows {
		out.Banners = append(out.Banners, bannerMsg(b))
	}
	return out, nil
}

func (s *BannerServer) CreateBanner(ctx context.Context, req *bannersv1.CreateBannerRequest) (*bannersv1.CreateBannerResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	in, err := bannerInput(req.GetBanner())
	if err != nil {
		return nil, err
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	b, err := s.svc.Create(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &bannersv1.CreateBannerResponse{Banner: bannerMsg(b)}, nil
}

func (s *BannerServer) UpdateBanner(ctx context.Context, req *bannersv1.UpdateBannerRequest) (*bannersv1.UpdateBannerResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	in, err := bannerInput(req.GetBanner())
	if err != nil {
		return nil, err
	}
	b, err := s.svc.Update(ctx, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &bannersv1.UpdateBannerResponse{Banner: bannerMsg(b)}, nil
}

func (s *BannerServer) SetBannerActive(ctx context.Context, req *bannersv1.SetBannerActiveRequest) (*bannersv1.SetBannerActiveResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetActive(ctx, id, req.GetActive()); err != nil {
		return nil, toStatus(err)
	}
	return &bannersv1.SetBannerActiveResponse{}, nil
}

func (s *BannerServer) DeleteBanner(ctx context.Context, req *bannersv1.DeleteBannerRequest) (*bannersv1.DeleteBannerResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &bannersv1.DeleteBannerResponse{}, nil
}

// ─────────────────────────────────────────────────────────────── leads

type LeadServer struct {
	leadsv1.UnimplementedLeadServiceServer
	svc *leads.Service
}

func NewLeadServer(svc *leads.Service) *LeadServer { return &LeadServer{svc: svc} }

func (s *LeadServer) CreateLead(ctx context.Context, req *leadsv1.CreateLeadRequest) (*leadsv1.CreateLeadResponse, error) {
	in := leads.CreateLeadInput{
		Name: req.GetName(), Phone: req.GetPhone(), Email: req.GetEmail(),
		InstituteName: req.GetInstituteName(), City: req.GetCity(),
		StudentsCount: int(req.GetStudentsCount()), Source: req.GetSource(),
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	row, err := s.svc.Create(ctx, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &leadsv1.CreateLeadResponse{Id: utils.UUIDFromPg(row.ID), Status: string(row.Status)}, nil
}

func (s *LeadServer) ListLeads(ctx context.Context, req *leadsv1.ListLeadsRequest) (*leadsv1.ListLeadsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if !c.Super {
		return nil, permDenied
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.List(ctx, req.GetStatus(), limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &leadsv1.ListLeadsResponse{}
	for _, l := range rows {
		out.Leads = append(out.Leads, &leadsv1.Lead{
			Id: utils.UUIDFromPg(l.ID), Name: utils.TextFromPg(l.Name), Phone: utils.TextFromPg(l.Phone),
			Email: utils.TextFromPg(l.Email), InstituteName: utils.TextFromPg(l.InstituteName), City: utils.TextFromPg(l.City),
			StudentsCount: l.StudentsCount.Int32, Source: utils.TextFromPg(l.Source), Status: string(l.Status),
			AssignedTo: utils.UUIDFromPg(l.AssignedTo), CreatedAt: tsFromPgtz(l.CreatedAt),
		})
	}
	return out, nil
}

func (s *LeadServer) UpdateLeadStatus(ctx context.Context, req *leadsv1.UpdateLeadStatusRequest) (*leadsv1.UpdateLeadStatusResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if !c.Super {
		return nil, permDenied
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	var assigned uuid.UUID
	if req.GetAssignedTo() != "" {
		assigned, err = uuid.Parse(req.GetAssignedTo())
		if err != nil {
			return nil, invalidArg("assigned_to must be a uuid")
		}
	}
	if err := s.svc.UpdateStatus(ctx, id, req.GetStatus(), assigned, req.GetNotes()); err != nil {
		return nil, toStatus(err)
	}
	return &leadsv1.UpdateLeadStatusResponse{}, nil
}

// ────────────────────────────────────────────────────────────── schedule

type ScheduleServer struct {
	schedulev1.UnimplementedScheduleServiceServer
	svc *schedule.Service
}

func NewScheduleServer(svc *schedule.Service) *ScheduleServer { return &ScheduleServer{svc: svc} }

func weekdaysToInt16(in []int32) []int16 {
	out := make([]int16, len(in))
	for i, v := range in {
		out[i] = int16(v)
	}
	return out
}

func weekdaysToInt32(in []int16) []int32 {
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v)
	}
	return out
}

func (s *ScheduleServer) ListSchedules(ctx context.Context, _ *schedulev1.ListSchedulesRequest) (*schedulev1.ListSchedulesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	rows, err := s.svc.List(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &schedulev1.ListSchedulesResponse{}
	for _, r := range rows {
		sch := &schedulev1.Schedule{
			Id: utils.UUIDFromPg(r.ID), CourseId: utils.UUIDFromPg(r.CourseID), BatchId: utils.UUIDFromPg(r.BatchID),
			InstructorId: utils.UUIDFromPg(r.InstructorID), Title: r.Title, ByWeekday: weekdaysToInt32(r.ByWeekday),
			StartLocal: r.StartLocal, DurationMin: r.DurationMin, Timezone: r.Timezone, IsActive: r.IsActive,
		}
		if r.StartsOn.Valid {
			sch.StartsOn = r.StartsOn.Time.Format("2006-01-02")
		}
		if r.EndsOn.Valid {
			sch.EndsOn = r.EndsOn.Time.Format("2006-01-02")
		}
		out.Schedules = append(out.Schedules, sch)
	}
	return out, nil
}

func (s *ScheduleServer) CreateSchedule(ctx context.Context, req *schedulev1.CreateScheduleRequest) (*schedulev1.CreateScheduleResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	in := schedule.CreateInput{
		InstructorID: req.GetInstructorId(), CourseID: req.GetCourseId(), BatchID: req.GetBatchId(),
		Title: req.GetTitle(), Description: req.GetDescription(), ByWeekday: weekdaysToInt16(req.GetByWeekday()),
		StartLocal: req.GetStartLocal(), DurationMin: req.GetDurationMin(), Timezone: req.GetTimezone(),
		StartsOn: req.GetStartsOn(), EndsOn: req.GetEndsOn(),
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	r, err := s.svc.Create(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &schedulev1.CreateScheduleResponse{Schedule: &schedulev1.Schedule{
		Id: utils.UUIDFromPg(r.ID), Title: r.Title, ByWeekday: weekdaysToInt32(r.ByWeekday),
		StartLocal: r.StartLocal, DurationMin: r.DurationMin, Timezone: r.Timezone, IsActive: r.IsActive,
	}}, nil
}

func (s *ScheduleServer) SetScheduleActive(ctx context.Context, req *schedulev1.SetScheduleActiveRequest) (*schedulev1.SetScheduleActiveResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetActive(ctx, id, req.GetActive()); err != nil {
		return nil, toStatus(err)
	}
	return &schedulev1.SetScheduleActiveResponse{}, nil
}

func (s *ScheduleServer) DeleteSchedule(ctx context.Context, req *schedulev1.DeleteScheduleRequest) (*schedulev1.DeleteScheduleResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &schedulev1.DeleteScheduleResponse{}, nil
}
