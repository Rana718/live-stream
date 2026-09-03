package grpcserver

import (
	"context"
	"time"

	commonv1 "live-platform/gen/proto/live/common/v1"
	platformadminv1 "live-platform/gen/proto/live/platformadmin/v1"
	"live-platform/internal/platformadmin"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PlatformAdminServer struct {
	platformadminv1.UnimplementedPlatformAdminServiceServer
	svc       *platformadmin.Service
	jwtSecret string
}

func NewPlatformAdminServer(svc *platformadmin.Service, jwtSecret string) *PlatformAdminServer {
	return &PlatformAdminServer{svc: svc, jwtSecret: jwtSecret}
}

func (s *PlatformAdminServer) super(ctx context.Context) error {
	c, ok := callerFrom(ctx)
	if !ok {
		return permDenied
	}
	if !c.Super {
		return permDenied
	}
	return nil
}

func (s *PlatformAdminServer) ListTenants(ctx context.Context, req *platformadminv1.ListTenantsRequest) (*platformadminv1.ListTenantsResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListTenants(ctx, req.GetStatus(), limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &platformadminv1.ListTenantsResponse{}
	for _, t := range rows {
		out.Tenants = append(out.Tenants, &platformadminv1.Tenant{
			Id: utils.UUIDFromPg(t.ID), OrgCode: t.OrgCode, Name: t.Name, Slug: t.Slug,
			Status: string(t.Status), Plan: string(t.Plan), BillingEmail: utils.TextFromPg(t.BillingEmail),
			RazorpayAccountId: utils.TextFromPg(t.RazorpayAccountID), TrialEndsAt: tsFromPgtz(t.TrialEndsAt),
			MemberCount: t.MemberCount,
		})
	}
	return out, nil
}

func (s *PlatformAdminServer) SuspendTenant(ctx context.Context, req *platformadminv1.SuspendTenantRequest) (*platformadminv1.SuspendTenantResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetTenantId(), "tenant_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SuspendTenant(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.SuspendTenantResponse{}, nil
}

func (s *PlatformAdminServer) ReactivateTenant(ctx context.Context, req *platformadminv1.ReactivateTenantRequest) (*platformadminv1.ReactivateTenantResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetTenantId(), "tenant_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.ReactivateTenant(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.ReactivateTenantResponse{}, nil
}

func (s *PlatformAdminServer) UpdateTenantPlan(ctx context.Context, req *platformadminv1.UpdateTenantPlanRequest) (*platformadminv1.UpdateTenantPlanResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetTenantId(), "tenant_id")
	if err != nil {
		return nil, err
	}
	var trial *time.Time
	if req.GetTrialEndsAt() != nil {
		t := req.GetTrialEndsAt().AsTime()
		trial = &t
	}
	if _, err := s.svc.UpdateTenantPlan(ctx, id, req.GetPlan(), req.GetStatus(), trial); err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.UpdateTenantPlanResponse{}, nil
}

func (s *PlatformAdminServer) SetTenantCustomDomain(ctx context.Context, req *platformadminv1.SetTenantCustomDomainRequest) (*platformadminv1.SetTenantCustomDomainResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetTenantId(), "tenant_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetCustomDomain(ctx, id, req.GetDomain()); err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.SetTenantCustomDomainResponse{}, nil
}

func (s *PlatformAdminServer) SetTenantRazorpayAccount(ctx context.Context, req *platformadminv1.SetTenantRazorpayAccountRequest) (*platformadminv1.SetTenantRazorpayAccountResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetTenantId(), "tenant_id")
	if err != nil {
		return nil, err
	}
	if _, err := s.svc.SetRazorpayAccount(ctx, id, req.GetAccountId()); err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.SetTenantRazorpayAccountResponse{}, nil
}

func (s *PlatformAdminServer) GetTenantFeatures(ctx context.Context, req *platformadminv1.GetTenantFeaturesRequest) (*platformadminv1.GetTenantFeaturesResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetTenantId(), "tenant_id")
	if err != nil {
		return nil, err
	}
	b, err := s.svc.GetFeatures(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.GetTenantFeaturesResponse{FeaturesJson: string(b)}, nil
}

func (s *PlatformAdminServer) SetTenantFeatures(ctx context.Context, req *platformadminv1.SetTenantFeaturesRequest) (*platformadminv1.SetTenantFeaturesResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetTenantId(), "tenant_id")
	if err != nil {
		return nil, err
	}
	b, err := s.svc.SetFeatures(ctx, id, []byte(req.GetFeaturesJson()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.SetTenantFeaturesResponse{FeaturesJson: string(b)}, nil
}

func (s *PlatformAdminServer) ListPayments(ctx context.Context, req *platformadminv1.ListPaymentsRequest) (*platformadminv1.ListPaymentsResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	var tenantID *uuid.UUID
	if req.GetTenantId() != "" {
		id, perr := uuid.Parse(req.GetTenantId())
		if perr != nil {
			return nil, invalidArg("tenant_id must be a uuid")
		}
		tenantID = &id
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListPayments(ctx, req.GetStatus(), tenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &platformadminv1.ListPaymentsResponse{}
	for _, p := range rows {
		method := ""
		if p.Method.Valid {
			method = string(p.Method.PaymentMethod)
		}
		out.Payments = append(out.Payments, &platformadminv1.Payment{
			Id: utils.UUIDFromPg(p.ID), TenantId: utils.UUIDFromPg(p.TenantID), OrderId: utils.UUIDFromPg(p.OrderID),
			OrderCode: p.OrderCode, UserId: utils.UUIDFromPg(p.UserID), FullName: utils.TextFromPg(p.FullName),
			Gateway: p.Gateway, GatewayPaymentId: utils.TextFromPg(p.GatewayPaymentID), Method: method,
			Status: string(p.Status), Amount: &commonv1.Money{Minor: p.AmountMinor, Currency: p.Currency},
			CapturedAt: tsFromPgtz(p.CapturedAt),
		})
	}
	return out, nil
}

func (s *PlatformAdminServer) ListUsers(ctx context.Context, req *platformadminv1.ListUsersRequest) (*platformadminv1.ListUsersResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	f := platformadmin.UserFilter{Role: req.GetRole(), Query: req.GetQuery()}
	if req.GetTenantId() != "" {
		id, perr := uuid.Parse(req.GetTenantId())
		if perr != nil {
			return nil, invalidArg("tenant_id must be a uuid")
		}
		f.TenantID = &id
	}
	limit, offset := pageArgs(req.GetPage())
	rows, total, err := s.svc.ListUsers(ctx, f, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &platformadminv1.ListUsersResponse{Total: total}
	for _, u := range rows {
		out.Users = append(out.Users, &platformadminv1.PlatformUser{
			Id: utils.UUIDFromPg(u.ID), FullName: utils.TextFromPg(u.FullName), Email: utils.TextFromPg(u.Email),
			Phone: utils.TextFromPg(u.Phone), Status: u.Status, Role: string(u.Role),
			TenantId: utils.UUIDFromPg(u.TenantID), OrgCode: u.OrgCode, TenantName: u.TenantName,
			CreatedAt: tsFromPgtz(u.CreatedAt),
		})
	}
	return out, nil
}

func (s *PlatformAdminServer) SetUserActive(ctx context.Context, req *platformadminv1.SetUserActiveRequest) (*platformadminv1.SetUserActiveResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetUserActive(ctx, id, req.GetActive()); err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.SetUserActiveResponse{}, nil
}

func (s *PlatformAdminServer) SetMembershipRole(ctx context.Context, req *platformadminv1.SetMembershipRoleRequest) (*platformadminv1.SetMembershipRoleResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	tenantID, err := parseUUID(req.GetTenantId(), "tenant_id")
	if err != nil {
		return nil, err
	}
	userID, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetMembershipRole(ctx, tenantID, userID, req.GetRole()); err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.SetMembershipRoleResponse{}, nil
}

func (s *PlatformAdminServer) PlatformStats(ctx context.Context, _ *platformadminv1.PlatformStatsRequest) (*platformadminv1.PlatformStatsResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	st, err := s.svc.PlatformStats(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.PlatformStatsResponse{
		TotalTenants: st.TotalTenants, ActiveTenants: st.ActiveTenants, TrialTenants: st.TrialTenants,
		SuspendedTenants: st.SuspendedTenants, TotalUsers: st.TotalUsers, TotalMemberships: st.TotalMemberships,
		TotalCourses: st.TotalCourses, LiveSessionsNow: st.LiveSessionsNow, MrrMinor: st.MrrMinor,
	}, nil
}

func (s *PlatformAdminServer) LeadStats(ctx context.Context, _ *platformadminv1.LeadStatsRequest) (*platformadminv1.LeadStatsResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	st, err := s.svc.LeadStats(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.LeadStatsResponse{
		TotalLeads: st.TotalLeads, NewLeads: st.NewLeads, ContactedLeads: st.ContactedLeads,
		QualifiedLeads: st.QualifiedLeads, ConvertedLeads: st.ConvertedLeads, LostLeads: st.LostLeads,
	}, nil
}

func (s *PlatformAdminServer) RecentSignups(ctx context.Context, req *platformadminv1.RecentSignupsRequest) (*platformadminv1.RecentSignupsResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	limit := req.GetLimit()
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.svc.RecentSignups(ctx, limit)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &platformadminv1.RecentSignupsResponse{}
	for _, u := range rows {
		out.Signups = append(out.Signups, &platformadminv1.Signup{
			Id: utils.UUIDFromPg(u.ID), FullName: utils.TextFromPg(u.FullName), Email: utils.TextFromPg(u.Email),
			Phone: utils.TextFromPg(u.Phone), Role: string(u.Role), OrgCode: u.OrgCode, TenantName: u.TenantName,
			CreatedAt: tsFromPgtz(u.CreatedAt),
		})
	}
	return out, nil
}

func (s *PlatformAdminServer) Impersonate(ctx context.Context, req *platformadminv1.ImpersonateRequest) (*platformadminv1.ImpersonateResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetTenantId(), "tenant_id")
	if err != nil {
		return nil, err
	}
	res, err := s.svc.Impersonate(ctx, id, s.jwtSecret)
	if err != nil {
		return nil, toStatus(err)
	}
	return &platformadminv1.ImpersonateResponse{
		AccessToken: res.AccessToken, TenantId: res.TenantID.String(), TenantName: res.TenantName,
		OrgCode: res.OrgCode, UserId: res.UserID.String(), ExpiresAt: timestamppb.New(res.ExpiresAt),
	}, nil
}
