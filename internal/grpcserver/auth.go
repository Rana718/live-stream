package grpcserver

import (
	"context"

	authv1 "live-platform/gen/proto/live/auth/v1"
	"live-platform/internal/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type AuthServer struct {
	authv1.UnimplementedAuthServiceServer
	svc *auth.Service
}

func NewAuthServer(svc *auth.Service) *AuthServer { return &AuthServer{svc: svc} }

func clientMeta(ctx context.Context) (userAgent, ip string) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("user-agent"); len(v) > 0 {
			userAgent = v[0]
		}
		if v := md.Get("x-forwarded-for"); len(v) > 0 {
			ip = v[0]
		}
	}
	if ip == "" {
		if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
			ip = p.Addr.String()
		}
	}
	return userAgent, ip
}

func tokenBundleMsg(b *auth.TokenBundle) *authv1.TokenBundle {
	if b == nil {
		return nil
	}
	return &authv1.TokenBundle{
		AccessToken: b.AccessToken, RefreshToken: b.RefreshToken, ExpiresIn: int32(b.ExpiresIn),
		User: &authv1.UserInfo{
			Id: b.User.ID.String(), Email: b.User.Email, Phone: b.User.Phone, FullName: b.User.FullName,
			Role: b.User.Role, TenantId: b.User.TenantID.String(), IsPlatformSuperAdmin: b.User.IsPlatformSuperAdmin,
		},
	}
}

func mapAuthErr(err error) error {
	if err == nil {
		return nil
	}
	// The auth layer reports most failures as opaque strings; surface them as
	// Unauthenticated (bad credentials) unless clearly a validation problem.
	return status.Error(codes.Unauthenticated, err.Error())
}

func (s *AuthServer) SendOtp(ctx context.Context, req *authv1.SendOtpRequest) (*authv1.SendOtpResponse, error) {
	if req.GetPhone() == "" {
		return nil, invalidArg("phone is required")
	}
	_, dev, err := s.svc.SendOTP(ctx, req.GetPhone(), req.GetOrgCode())
	if err != nil {
		return nil, mapAuthErr(err)
	}
	return &authv1.SendOtpResponse{Sent: true, DevCode: dev}, nil
}

func (s *AuthServer) VerifyOtp(ctx context.Context, req *authv1.VerifyOtpRequest) (*authv1.VerifyOtpResponse, error) {
	ua, ip := clientMeta(ctx)
	b, err := s.svc.VerifyOTP(ctx, req.GetPhone(), req.GetCode(), req.GetOrgCode(), req.GetReferralCode(), ua, ip)
	if err != nil {
		return nil, mapAuthErr(err)
	}
	return &authv1.VerifyOtpResponse{Tokens: tokenBundleMsg(b)}, nil
}

func (s *AuthServer) GoogleLogin(ctx context.Context, req *authv1.GoogleLoginRequest) (*authv1.GoogleLoginResponse, error) {
	ua, ip := clientMeta(ctx)
	b, err := s.svc.GoogleLogin(ctx, req.GetIdToken(), req.GetOrgCode(), req.GetReferralCode(), ua, ip)
	if err != nil {
		return nil, mapAuthErr(err)
	}
	return &authv1.GoogleLoginResponse{Tokens: tokenBundleMsg(b)}, nil
}

func (s *AuthServer) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	if req.GetRefreshToken() == "" {
		return nil, invalidArg("refresh_token is required")
	}
	ua, ip := clientMeta(ctx)
	b, err := s.svc.Refresh(ctx, req.GetRefreshToken(), ua, ip)
	if err != nil {
		return nil, mapAuthErr(err)
	}
	return &authv1.RefreshTokenResponse{Tokens: tokenBundleMsg(b)}, nil
}

func (s *AuthServer) Logout(ctx context.Context, req *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	if err := s.svc.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, toStatus(err)
	}
	return &authv1.LogoutResponse{}, nil
}

func (s *AuthServer) SwitchOrg(ctx context.Context, req *authv1.SwitchOrgRequest) (*authv1.SwitchOrgResponse, error) {
	c, ok := callerFrom(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing bearer token")
	}
	targetID, perr := parseUUID(req.GetTargetTenantId(), "target_tenant_id")
	if perr != nil {
		return nil, perr
	}
	ua, ip := clientMeta(ctx)
	b, aerr := s.svc.SwitchOrg(ctx, c.UserID, targetID, ua, ip)
	if aerr != nil {
		return nil, mapAuthErr(aerr)
	}
	return &authv1.SwitchOrgResponse{Tokens: tokenBundleMsg(b)}, nil
}

func (s *AuthServer) LinkPhone(ctx context.Context, req *authv1.LinkPhoneRequest) (*authv1.LinkPhoneResponse, error) {
	c, ok := callerFrom(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing bearer token")
	}
	if err := s.svc.LinkPhone(ctx, c.UserID, req.GetPhone(), req.GetCode(), req.GetOrgCode()); err != nil {
		return nil, mapAuthErr(err)
	}
	return &authv1.LinkPhoneResponse{}, nil
}

func (s *AuthServer) LinkGoogle(ctx context.Context, req *authv1.LinkGoogleRequest) (*authv1.LinkGoogleResponse, error) {
	c, ok := callerFrom(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing bearer token")
	}
	if err := s.svc.LinkGoogle(ctx, c.UserID, req.GetIdToken()); err != nil {
		return nil, mapAuthErr(err)
	}
	return &authv1.LinkGoogleResponse{}, nil
}
