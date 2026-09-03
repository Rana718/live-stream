// Package grpcserver hosts the gRPC API surface. Every service here is a thin
// transport adapter over the SAME internal/* service layer the REST handlers
// use — no business logic is duplicated. Auth is enforced by authInterceptor,
// which mirrors the REST middleware: it validates the bearer token from
// metadata, puts tenant/user/role in the context, and opens the RLS scope via
// database.WithTenant / WithSuperAdmin.
package grpcserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"strings"
	"time"

	"live-platform/internal/aiclient"
	"live-platform/internal/auth/google"
	"live-platform/internal/config"
	"live-platform/internal/database"
	"live-platform/internal/email"
	"live-platform/internal/events"
	"live-platform/internal/payments"
	"live-platform/internal/sms"
	"live-platform/internal/storage"
	"live-platform/internal/utils"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// Deps carries the shared client instances main() already constructed, so the
// gRPC server reuses them (one Kafka producer, one Redis client, …) rather
// than opening its own.
type Deps struct {
	Pool      *pgxpool.Pool
	Redis     *redis.Client
	Kafka     *events.Producer
	Razorpay  *payments.Razorpay
	MinIO     *storage.MinIOClient
	Claude    *aiclient.Claude
	SMS       sms.Client
	Email     email.Client
	Google    *google.Verifier
	Codemagic config.CodemagicConfig
	Log       *slog.Logger
}

// New builds a *grpc.Server with every domain service registered and the
// interceptor chain installed. Serve it with Start.
func New(cfg *config.Config, d Deps) *grpc.Server {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(
		recoveryInterceptor(d.Log),
		logInterceptor(d.Log),
		authInterceptor(cfg),
	))
	registerAll(s, cfg, d)
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

// publicMethods skip token validation entirely (login / OTP / refresh).
var publicMethods = map[string]bool{
	"/live.auth.v1.AuthService/SendOtp":      true,
	"/live.auth.v1.AuthService/VerifyOtp":    true,
	"/live.auth.v1.AuthService/GoogleLogin":  true,
	"/live.auth.v1.AuthService/RefreshToken": true,
	"/live.auth.v1.AuthService/ResolveOrg":   true,
	"/live.leads.v1.LeadService/CreateLead":  true,
}

// optionalAuthMethods accept an anonymous caller — with no token they run
// under database.WithPublicLookup (the public catalog surface). With a token
// they run fully scoped like any other method.
var optionalAuthMethods = map[string]bool{
	"/live.courses.v1.CourseService/ListCourses":             true,
	"/live.courses.v1.CourseService/GetCourse":               true,
	"/live.courses.v1.CourseService/SearchCourses":           true,
	"/live.exams.v1.ExamCategoryService/ListExamCategories":  true,
	"/live.exams.v1.ExamCategoryService/GetExamCategory":     true,
	"/live.subjects.v1.SubjectService/ListSubjects":          true,
	"/live.subjects.v1.SubjectService/GetSubject":            true,
	"/live.chapters.v1.ChapterService/ListChaptersBySubject": true,
	"/live.chapters.v1.ChapterService/GetChapter":            true,
	"/live.topics.v1.TopicService/ListTopicsByChapter":       true,
	"/live.topics.v1.TopicService/GetTopic":                  true,
	"/live.lectures.v1.LectureService/ListLectures":          true,
	"/live.lectures.v1.LectureService/GetLecture":            true,
	"/live.lectures.v1.LectureService/ListSections":          true,
	"/live.batches.v1.BatchService/ListBatchesByCourse":      true,
	"/live.batches.v1.BatchService/GetBatch":                 true,
}

// authInterceptor mirrors internal/middleware auth + tenant context.
func authInterceptor(cfg *config.Config) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		raw := bearerFrom(ctx)
		if raw == "" {
			if optionalAuthMethods[info.FullMethod] {
				return handler(database.WithPublicLookup(ctx), req)
			}
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

func bearerFrom(ctx context.Context) string {
	md, _ := metadata.FromIncomingContext(ctx)
	v := md.Get("authorization")
	if len(v) == 0 {
		return ""
	}
	raw := strings.TrimSpace(v[0])
	raw = strings.TrimPrefix(raw, "Bearer ")
	raw = strings.TrimPrefix(raw, "bearer ")
	return strings.TrimSpace(raw)
}

// recoveryInterceptor turns a panic into codes.Internal (REST has
// middleware.Recovery; gRPC needs its own).
func recoveryInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("grpc panic", "method", info.FullMethod, "panic", r, "stack", string(debug.Stack()))
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

// logInterceptor records method, code and duration — mirrors the REST request logger.
func logInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		log.Info("grpc call",
			"method", info.FullMethod,
			"code", status.Code(err).String(),
			"dur_ms", time.Since(start).Milliseconds(),
		)
		return resp, err
	}
}
