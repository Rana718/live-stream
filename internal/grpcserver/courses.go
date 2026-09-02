// Package grpcserver hosts the gRPC API surface. Each service here is a thin
// transport adapter over the SAME internal service layer the REST handlers
// use — no business logic is duplicated. Auth is enforced by the
// authInterceptor (see server.go), which mirrors the REST middleware: it
// validates the bearer token from metadata and puts tenant/user/role into
// the context, and it opens the RLS scope via database.WithTenant.
package grpcserver

import (
	"context"

	pb "live-platform/gen/proto/live/v1"
	"live-platform/internal/courses"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CourseServer struct {
	pb.UnimplementedCourseServiceServer
	svc *courses.Service
}

func NewCourseServer(svc *courses.Service) *CourseServer { return &CourseServer{svc: svc} }

func (s *CourseServer) ListCourses(ctx context.Context, req *pb.ListCoursesRequest) (*pb.ListCoursesResponse, error) {
	tenantID := tenantFrom(ctx)
	if tenantID == uuid.Nil {
		return nil, status.Error(codes.Unauthenticated, "missing tenant context")
	}
	limit, offset := req.GetLimit(), req.GetOffset()
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	out := &pb.ListCoursesResponse{}
	if req.GetIncludeUnpublished() && roleAllows(ctx, "owner", "admin", "instructor") {
		rows, err := s.svc.ListForAdmin(ctx, tenantID, limit, offset)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		for _, r := range rows {
			out.Courses = append(out.Courses, &pb.Course{
				Id: utils.UUIDFromPg(r.ID), Title: r.Title, Slug: r.Slug,
				Summary: utils.TextFromPg(r.Summary), ThumbnailUrl: utils.TextFromPg(r.ThumbnailUrl),
				Language: r.Language, Level: r.Level,
				Status: string(r.Status), IsPublished: r.Status == db.PublishStatusPublished,
				PriceMinor: r.PriceMinor, Currency: "INR",
			})
		}
		return out, nil
	}
	rows, err := s.svc.ListPublished(ctx, tenantID, limit, offset)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	for _, r := range rows {
		out.Courses = append(out.Courses, &pb.Course{
			Id: utils.UUIDFromPg(r.ID), Title: r.Title, Slug: r.Slug,
			Summary: utils.TextFromPg(r.Summary), ThumbnailUrl: utils.TextFromPg(r.ThumbnailUrl),
			Language: r.Language, Level: r.Level, Status: "published", IsPublished: true,
			PriceMinor: r.PriceMinor, Currency: "INR",
		})
	}
	return out, nil
}

func (s *CourseServer) GetCourse(ctx context.Context, req *pb.GetCourseRequest) (*pb.GetCourseResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "id must be a uuid")
	}
	c, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "course not found")
	}
	price, _ := s.svc.GetPrice(ctx, id)
	return &pb.GetCourseResponse{Course: &pb.Course{
		Id: utils.UUIDFromPg(c.ID), Title: c.Title, Slug: c.Slug,
		Summary: utils.TextFromPg(c.Summary), ThumbnailUrl: utils.TextFromPg(c.ThumbnailUrl),
		Language: c.Language, Level: c.Level, Status: string(c.Status),
		IsPublished: c.Status == db.PublishStatusPublished, PriceMinor: price, Currency: "INR",
	}}, nil
}

func (s *CourseServer) CreateCourse(ctx context.Context, req *pb.CreateCourseRequest) (*pb.CreateCourseResponse, error) {
	tenantID, userID := tenantFrom(ctx), userFrom(ctx)
	if tenantID == uuid.Nil {
		return nil, status.Error(codes.Unauthenticated, "missing tenant context")
	}
	if !roleAllows(ctx, "owner", "admin", "instructor") {
		return nil, status.Error(codes.PermissionDenied, "instructor or admin only")
	}
	row, err := s.svc.Create(ctx, tenantID, userID, courses.CreateCourseRequest{
		Title: req.GetTitle(), Summary: req.GetSummary(),
		Language: req.GetLanguage(), Level: req.GetLevel(),
		PriceMinor: req.GetPriceMinor(), HsnSac: req.GetHsnSac(),
		TaxRateBps: req.GetTaxRateBps(),
	})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &pb.CreateCourseResponse{Course: &pb.Course{
		Id: utils.UUIDFromPg(row.ID), Title: row.Title, Slug: row.Slug,
		Status: string(row.Status), PriceMinor: req.GetPriceMinor(), Currency: "INR",
	}}, nil
}
