package grpcserver

import (
	"context"
	"fmt"
	"net"
	"strings"

	pb "live-platform/gen/proto/live/v1"
	"live-platform/internal/config"
	"live-platform/internal/courses"
	"live-platform/internal/database"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type ctxKey int

const (
	ctxTenant ctxKey = iota
	ctxUser
	ctxRole
)

func tenantFrom(ctx context.Context) uuid.UUID { id, _ := ctx.Value(ctxTenant).(uuid.UUID); return id }
func userFrom(ctx context.Context) uuid.UUID   { id, _ := ctx.Value(ctxUser).(uuid.UUID); return id }

func roleAllows(ctx context.Context, allowed ...string) bool {
	role, _ := ctx.Value(ctxRole).(string)
	if role == "super_admin" {
		return true
	}
	for _, a := range allowed {
		if a == role {
			return true
		}
	}
	return false
}

// New builds a *grpc.Server with every domain service registered and the
// auth interceptor installed. Serve it with Start.
func New(cfg *config.Config, pool *pgxpool.Pool) *grpc.Server {
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(authInterceptor(cfg)))
	pb.RegisterCourseServiceServer(s, NewCourseServer(courses.NewService(pool)))
	// Register further domain servers here as their protos land.
	if cfg.Server.Env != "production" {
		reflection.Register(s) // enables grpcurl / grpc_cli without the .proto
	}
	return s
}

// Start listens on port and blocks until ctx is cancelled.
func Start(ctx context.Context, s *grpc.Server, port string) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}
	go func() {
		<-ctx.Done()
		s.GracefulStop()
	}()
	return s.Serve(lis)
}

// authInterceptor mirrors the REST middleware: pull the bearer token from
// the "authorization" metadata key, validate it, and stamp tenant/user/role
// into the context plus open the RLS scope. Methods that need no auth would
// be allowlisted here; today every course RPC is authenticated.
func authInterceptor(cfg *config.Config) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		var raw string
		if v := md.Get("authorization"); len(v) > 0 {
			raw = strings.TrimPrefix(v[0], "Bearer ")
			raw = strings.TrimPrefix(raw, "bearer ")
		}
		if raw == "" {
			return nil, status.Error(codes.Unauthenticated, "missing bearer token")
		}
		claims, err := utils.ValidateToken(raw, cfg.JWT.AccessSecret)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}
		ctx = context.WithValue(ctx, ctxTenant, claims.TenantID)
		ctx = context.WithValue(ctx, ctxUser, claims.UserID)
		ctx = context.WithValue(ctx, ctxRole, claims.Role)
		if claims.Role == "super_admin" {
			ctx = database.WithSuperAdmin(ctx)
		} else {
			ctx = database.WithTenant(ctx, claims.TenantID.String(), claims.UserID.String())
		}
		return handler(ctx, req)
	}
}
