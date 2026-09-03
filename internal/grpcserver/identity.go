package grpcserver

import (
	"context"

	adminv1 "live-platform/gen/proto/live/admin/v1"
	auditv1 "live-platform/gen/proto/live/audit/v1"
	devicesv1 "live-platform/gen/proto/live/devices/v1"
	usersv1 "live-platform/gen/proto/live/users/v1"
	"live-platform/internal/admin"
	"live-platform/internal/audit"
	"live-platform/internal/devices"
	"live-platform/internal/users"
	"live-platform/internal/utils"
)

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ─────────────────────────────────────────────────────────────── users

type UserServer struct {
	usersv1.UnimplementedUserServiceServer
	svc *users.Service
}

func NewUserServer(svc *users.Service) *UserServer { return &UserServer{svc: svc} }

func (s *UserServer) GetMyProfile(ctx context.Context, _ *usersv1.GetMyProfileRequest) (*usersv1.GetMyProfileResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.svc.GetProfile(ctx, c.TenantID, c.UserID, c.Role)
	if err != nil {
		return nil, toStatus(err)
	}
	return &usersv1.GetMyProfileResponse{Profile: &usersv1.Profile{
		Id: p.ID.String(), Email: p.Email, Phone: p.Phone, FullName: p.FullName, AvatarUrl: p.AvatarURL,
		Role: p.Role, TenantId: p.TenantID.String(), ClassLevel: deref(p.ClassLevel), Board: deref(p.Board),
		ExamGoal: deref(p.ExamGoal), GuardianName: deref(p.GuardianName), GuardianPhone: deref(p.GuardianPhone),
		OnboardingCompleted: p.OnboardingCompleted,
	}}, nil
}

func (s *UserServer) UpdateMyBasics(ctx context.Context, req *usersv1.UpdateMyBasicsRequest) (*usersv1.UpdateMyBasicsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.svc.UpdateBasics(ctx, c.UserID, req.GetFullName(), req.GetAvatarUrl()); err != nil {
		return nil, toStatus(err)
	}
	return &usersv1.UpdateMyBasicsResponse{}, nil
}

func (s *UserServer) CompleteOnboarding(ctx context.Context, req *usersv1.CompleteOnboardingRequest) (*usersv1.CompleteOnboardingResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	in := users.OnboardingInput{
		FullName: req.GetFullName(), ClassLevel: req.GetClassLevel(), Board: req.GetBoard(),
		ExamGoal: req.GetExamGoal(), GuardianName: req.GetGuardianName(), GuardianPhone: req.GetGuardianPhone(),
	}
	if err := s.svc.CompleteOnboarding(ctx, c.TenantID, c.UserID, in); err != nil {
		return nil, toStatus(err)
	}
	return &usersv1.CompleteOnboardingResponse{}, nil
}

func (s *UserServer) ListMembers(ctx context.Context, req *usersv1.ListMembersRequest) (*usersv1.ListMembersResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListMembers(ctx, c.TenantID, req.GetRole(), limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &usersv1.ListMembersResponse{}
	for _, m := range rows {
		out.Members = append(out.Members, &usersv1.Member{
			UserId: utils.UUIDFromPg(m.UserID), FullName: utils.TextFromPg(m.FullName),
			Email: utils.TextFromPg(m.Email), Phone: utils.TextFromPg(m.Phone),
			Role: string(m.Role), Status: string(m.Status), JoinedAt: tsFromPgtz(m.JoinedAt),
		})
	}
	return out, nil
}

// ─────────────────────────────────────────────────────────────── devices

type DeviceServer struct {
	devicesv1.UnimplementedDeviceServiceServer
	svc *devices.Service
}

func NewDeviceServer(svc *devices.Service) *DeviceServer { return &DeviceServer{svc: svc} }

func (s *DeviceServer) RegisterDevice(ctx context.Context, req *devicesv1.RegisterDeviceRequest) (*devicesv1.RegisterDeviceResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	in := devices.RegisterInput{Token: req.GetToken(), Platform: req.GetPlatform()}
	if err := validate(&in); err != nil {
		return nil, err
	}
	if err := s.svc.Register(ctx, c.TenantID, c.UserID, in); err != nil {
		return nil, toStatus(err)
	}
	return &devicesv1.RegisterDeviceResponse{}, nil
}

func (s *DeviceServer) UnregisterDevice(ctx context.Context, req *devicesv1.UnregisterDeviceRequest) (*devicesv1.UnregisterDeviceResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	if err := s.svc.Unregister(ctx, req.GetToken()); err != nil {
		return nil, toStatus(err)
	}
	return &devicesv1.UnregisterDeviceResponse{}, nil
}

func (s *DeviceServer) ListMyDeviceTokens(ctx context.Context, _ *devicesv1.ListMyDeviceTokensRequest) (*devicesv1.ListMyDeviceTokensResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	toks, err := s.svc.TokensForUser(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &devicesv1.ListMyDeviceTokensResponse{Tokens: toks}, nil
}

// ─────────────────────────────────────────────────────────────── audit

type AuditServer struct {
	auditv1.UnimplementedAuditServiceServer
	svc *audit.Service
}

func NewAuditServer(svc *audit.Service) *AuditServer { return &AuditServer{svc: svc} }

func (s *AuditServer) ListAuditLogs(ctx context.Context, req *auditv1.ListAuditLogsRequest) (*auditv1.ListAuditLogsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.List(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &auditv1.ListAuditLogsResponse{}
	for _, l := range rows {
		out.Logs = append(out.Logs, &auditv1.AuditLog{
			Id: utils.UUIDFromPg(l.ID), ActorUserId: utils.UUIDFromPg(l.ActorUserID),
			ActorRole: utils.TextFromPg(l.ActorRole), Action: l.Action,
			EntityType: utils.TextFromPg(l.EntityType), EntityId: utils.UUIDFromPg(l.EntityID),
			CreatedAt: tsFromPgtz(l.CreatedAt),
		})
	}
	return out, nil
}

// ─────────────────────────────────────────────────────────────── admin

type AdminServer struct {
	adminv1.UnimplementedAdminServiceServer
	svc *admin.Service
}

func NewAdminServer(svc *admin.Service) *AdminServer { return &AdminServer{svc: svc} }

func (s *AdminServer) adminCaller(ctx context.Context) (caller, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return c, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return c, err
	}
	return c, nil
}

func (s *AdminServer) DashboardStats(ctx context.Context, _ *adminv1.DashboardStatsRequest) (*adminv1.DashboardStatsResponse, error) {
	c, err := s.adminCaller(ctx)
	if err != nil {
		return nil, err
	}
	st, err := s.svc.DashboardStats(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &adminv1.DashboardStatsResponse{Stats: &adminv1.DashboardStats{
		TotalCourses: st.TotalCourses, PublishedCourses: st.PublishedCourses, TotalStudents: st.TotalStudents,
		TotalInstructors: st.TotalInstructors, TotalEnrollments: st.TotalEnrollments,
		RevenueMinor: st.RevenueMinor, PaidOrders: st.PaidOrders,
	}}, nil
}

func (s *AdminServer) ListMembers(ctx context.Context, req *adminv1.ListMembersRequest) (*adminv1.ListMembersResponse, error) {
	c, err := s.adminCaller(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListMembers(ctx, c.TenantID, req.GetRole(), req.GetQuery(), limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &adminv1.ListMembersResponse{}
	for _, m := range rows {
		out.Members = append(out.Members, &adminv1.Member{
			UserId: utils.UUIDFromPg(m.ID), FullName: utils.TextFromPg(m.FullName), Email: utils.TextFromPg(m.Email),
			Phone: utils.TextFromPg(m.Phone), Role: string(m.Role), Status: m.Status,
			MembershipStatus: string(m.MembershipStatus), CreatedAt: tsFromPgtz(m.CreatedAt), LastLoginAt: tsFromPgtz(m.LastLoginAt),
		})
	}
	return out, nil
}

func (s *AdminServer) UpdateMember(ctx context.Context, req *adminv1.UpdateMemberRequest) (*adminv1.UpdateMemberResponse, error) {
	c, err := s.adminCaller(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	m, err := s.svc.UpdateUser(ctx, c.TenantID, id, admin.AdminUpdateUserRequest{
		FullName: req.GetFullName(), Email: req.GetEmail(), Phone: req.GetPhone(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &adminv1.UpdateMemberResponse{Member: &adminv1.Member{
		UserId: utils.UUIDFromPg(m.ID), FullName: utils.TextFromPg(m.FullName), Email: utils.TextFromPg(m.Email),
		Phone: utils.TextFromPg(m.Phone), Role: string(m.Role), Status: m.Status, MembershipStatus: string(m.MembershipStatus),
		CreatedAt: tsFromPgtz(m.CreatedAt),
	}}, nil
}

func (s *AdminServer) SetMemberRole(ctx context.Context, req *adminv1.SetMemberRoleRequest) (*adminv1.SetMemberRoleResponse, error) {
	c, err := s.adminCaller(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetUserRole(ctx, c.TenantID, id, req.GetRole()); err != nil {
		return nil, toStatus(err)
	}
	return &adminv1.SetMemberRoleResponse{}, nil
}

func (s *AdminServer) SetMemberActive(ctx context.Context, req *adminv1.SetMemberActiveRequest) (*adminv1.SetMemberActiveResponse, error) {
	c, err := s.adminCaller(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetUserActive(ctx, c.TenantID, id, req.GetActive()); err != nil {
		return nil, toStatus(err)
	}
	return &adminv1.SetMemberActiveResponse{}, nil
}

func (s *AdminServer) DeleteMember(ctx context.Context, req *adminv1.DeleteMemberRequest) (*adminv1.DeleteMemberResponse, error) {
	c, err := s.adminCaller(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeleteUser(ctx, c.TenantID, id); err != nil {
		return nil, toStatus(err)
	}
	return &adminv1.DeleteMemberResponse{}, nil
}

func (s *AdminServer) ListPendingCourses(ctx context.Context, req *adminv1.ListPendingCoursesRequest) (*adminv1.ListPendingCoursesResponse, error) {
	c, err := s.adminCaller(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListPendingApproval(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &adminv1.ListPendingCoursesResponse{}
	for _, r := range rows {
		out.Courses = append(out.Courses, &adminv1.PendingCourse{
			Id: utils.UUIDFromPg(r.ID), Title: r.Title, Slug: r.Slug,
			CreatedBy: utils.UUIDFromPg(r.CreatedBy), CreatedAt: tsFromPgtz(r.CreatedAt),
		})
	}
	return out, nil
}

func (s *AdminServer) ApproveCourse(ctx context.Context, req *adminv1.ApproveCourseRequest) (*adminv1.ApproveCourseResponse, error) {
	c, err := s.adminCaller(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	if _, err := s.svc.ApproveCourse(ctx, id, c.UserID); err != nil {
		return nil, toStatus(err)
	}
	return &adminv1.ApproveCourseResponse{}, nil
}

func (s *AdminServer) RejectCourse(ctx context.Context, req *adminv1.RejectCourseRequest) (*adminv1.RejectCourseResponse, error) {
	if _, err := s.adminCaller(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	if _, err := s.svc.RejectCourse(ctx, id, req.GetReason()); err != nil {
		return nil, toStatus(err)
	}
	return &adminv1.RejectCourseResponse{}, nil
}
