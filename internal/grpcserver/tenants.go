package grpcserver

import (
	"context"
	"encoding/json"

	tenantsv1 "live-platform/gen/proto/live/tenants/v1"
	"live-platform/internal/tenants"
)

type TenantServer struct {
	tenantsv1.UnimplementedTenantServiceServer
	svc *tenants.Service
}

func NewTenantServer(svc *tenants.Service) *TenantServer { return &TenantServer{svc: svc} }

func jsonStr(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (s *TenantServer) LookupByOrgCode(ctx context.Context, req *tenantsv1.LookupByOrgCodeRequest) (*tenantsv1.LookupByOrgCodeResponse, error) {
	info, err := s.svc.LookupByOrgCode(ctx, req.GetOrgCode())
	if err != nil {
		return nil, toStatus(err)
	}
	return &tenantsv1.LookupByOrgCodeResponse{Tenant: &tenantsv1.PublicTenant{
		Id: info.ID.String(), OrgCode: info.OrgCode, Name: info.Name, Slug: info.Slug,
		LogoUrl: info.LogoURL, ThemeJson: string(info.Theme), Status: info.Status,
	}}, nil
}

func (s *TenantServer) GetMyTenant(ctx context.Context, _ *tenantsv1.GetMyTenantRequest) (*tenantsv1.GetMyTenantResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	m, err := s.svc.MyTenant(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &tenantsv1.GetMyTenantResponse{TenantJson: jsonStr(m)}, nil
}

func (s *TenantServer) UpdateBranding(ctx context.Context, req *tenantsv1.UpdateBrandingRequest) (*tenantsv1.UpdateBrandingResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	in := tenants.UpdateBrandingRequest{Name: req.GetName(), LogoURL: req.GetLogoUrl()}
	if t := req.GetThemeJson(); t != "" {
		in.Theme = json.RawMessage(t)
	}
	m, err := s.svc.UpdateBranding(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &tenantsv1.UpdateBrandingResponse{TenantJson: jsonStr(m)}, nil
}

func (s *TenantServer) SelfServeOnboard(ctx context.Context, req *tenantsv1.SelfServeOnboardRequest) (*tenantsv1.SelfServeOnboardResponse, error) {
	res, err := s.svc.SelfServeOnboard(ctx, tenants.SelfServeOnboardRequest{
		OrgName: req.GetOrgName(), AdminName: req.GetAdminName(), AdminPhone: req.GetAdminPhone(),
		AdminEmail: req.GetAdminEmail(), City: req.GetCity(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &tenantsv1.SelfServeOnboardResponse{
		TenantId: res.TenantID.String(), OrgCode: res.OrgCode, Slug: res.Slug, AdminId: res.AdminID.String(),
	}, nil
}
