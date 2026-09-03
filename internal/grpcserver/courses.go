package grpcserver

import (
	"context"

	commonv1 "live-platform/gen/proto/live/common/v1"
	coursesv1 "live-platform/gen/proto/live/courses/v1"
	"live-platform/internal/courses"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CourseServer struct {
	coursesv1.UnimplementedCourseServiceServer
	svc *courses.Service
}

func NewCourseServer(svc *courses.Service) *CourseServer { return &CourseServer{svc: svc} }

func money(minor int64) *commonv1.Money {
	return &commonv1.Money{Minor: minor, Currency: "INR"}
}

func pageArgs(p *commonv1.PageRequest) (limit, offset int32) {
	if p == nil {
		return 20, 0
	}
	limit, offset = p.GetLimit(), p.GetOffset()
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (s *CourseServer) ListCourses(ctx context.Context, req *coursesv1.ListCoursesRequest) (*coursesv1.ListCoursesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	out := &coursesv1.ListCoursesResponse{Page: &commonv1.PageResponse{Limit: limit, Offset: offset, Total: -1}}

	if req.GetIncludeUnpublished() && c.require(rolesInstructorUp...) == nil {
		rows, err := s.svc.ListForAdmin(ctx, c.TenantID, limit, offset)
		if err != nil {
			return nil, toStatus(err)
		}
		for _, r := range rows {
			out.Courses = append(out.Courses, &coursesv1.Course{
				Id: utils.UUIDFromPg(r.ID), Title: r.Title, Slug: r.Slug,
				Summary: utils.TextFromPg(r.Summary), ThumbnailUrl: utils.TextFromPg(r.ThumbnailUrl),
				Language: r.Language, Level: r.Level, Status: string(r.Status),
				IsPublished: r.Status == db.PublishStatusPublished, ApprovalStatus: r.ApprovalStatus,
				Price: money(r.PriceMinor), CreatedAt: tsFromPgtz(r.CreatedAt),
			})
		}
		return out, nil
	}

	rows, err := s.svc.ListPublished(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	for _, r := range rows {
		out.Courses = append(out.Courses, &coursesv1.Course{
			Id: utils.UUIDFromPg(r.ID), Title: r.Title, Slug: r.Slug,
			Summary: utils.TextFromPg(r.Summary), ThumbnailUrl: utils.TextFromPg(r.ThumbnailUrl),
			Language: r.Language, Level: r.Level, Status: "published", IsPublished: true,
			ApprovalStatus: "approved", Price: money(r.PriceMinor),
		})
	}
	return out, nil
}

func (s *CourseServer) GetCourse(ctx context.Context, req *coursesv1.GetCourseRequest) (*coursesv1.GetCourseResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSlug() != "" && req.GetId() == "" {
		row, err := s.svc.GetBySlug(ctx, c.TenantID, req.GetSlug())
		if err != nil {
			return nil, toStatus(err)
		}
		price, _ := s.svc.GetPrice(ctx, uuid.UUID(row.ID.Bytes))
		return &coursesv1.GetCourseResponse{Course: &coursesv1.Course{
			Id: utils.UUIDFromPg(row.ID), Title: row.Title, Slug: row.Slug,
			Summary: utils.TextFromPg(row.Summary), ThumbnailUrl: utils.TextFromPg(row.ThumbnailUrl),
			Status: string(row.Status), IsPublished: row.Status == db.PublishStatusPublished,
			Price: money(price),
		}}, nil
	}

	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	row, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	price, _ := s.svc.GetPrice(ctx, id)
	return &coursesv1.GetCourseResponse{Course: &coursesv1.Course{
		Id: utils.UUIDFromPg(row.ID), Title: row.Title, Slug: row.Slug,
		Summary: utils.TextFromPg(row.Summary), ThumbnailUrl: utils.TextFromPg(row.ThumbnailUrl),
		Language: row.Language, Level: row.Level, Status: string(row.Status),
		IsPublished: row.Status == db.PublishStatusPublished, ApprovalStatus: row.ApprovalStatus,
		Price: money(price), CreatedAt: tsFromPgtz(row.CreatedAt),
	}}, nil
}

func (s *CourseServer) SearchCourses(ctx context.Context, req *coursesv1.SearchCoursesRequest) (*coursesv1.SearchCoursesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.Search(ctx, c.TenantID, req.GetQuery(), limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &coursesv1.SearchCoursesResponse{}
	for _, r := range rows {
		out.Courses = append(out.Courses, &coursesv1.Course{
			Id: utils.UUIDFromPg(r.ID), Title: r.Title, Slug: r.Slug,
			Summary: utils.TextFromPg(r.Summary), ThumbnailUrl: utils.TextFromPg(r.ThumbnailUrl),
		})
	}
	return out, nil
}

func (s *CourseServer) ListPendingCourses(ctx context.Context, req *coursesv1.ListPendingCoursesRequest) (*coursesv1.ListPendingCoursesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListPending(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &coursesv1.ListPendingCoursesResponse{}
	for _, r := range rows {
		out.Courses = append(out.Courses, &coursesv1.Course{
			Id: utils.UUIDFromPg(r.ID), Title: r.Title, Slug: r.Slug, ApprovalStatus: "pending",
			CreatedAt: tsFromPgtz(r.CreatedAt),
		})
	}
	return out, nil
}

func (s *CourseServer) CreateCourse(ctx context.Context, req *coursesv1.CreateCourseRequest) (*coursesv1.CreateCourseResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	examCat, err := optUUID(req.GetExamCategoryId(), "exam_category_id")
	if err != nil {
		return nil, err
	}
	in := courses.CreateCourseRequest{
		Title: req.GetTitle(), Summary: req.GetSummary(), Language: req.GetLanguage(),
		Level: req.GetLevel(), PriceMinor: req.GetPriceMinor(), HsnSac: req.GetHsnSac(),
		TaxRateBps: req.GetTaxRateBps(), ExamCategoryID: examCat,
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	row, err := s.svc.Create(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &coursesv1.CreateCourseResponse{Course: &coursesv1.Course{
		Id: utils.UUIDFromPg(row.ID), Title: row.Title, Slug: row.Slug,
		Status: string(row.Status), ApprovalStatus: row.ApprovalStatus, Price: money(req.GetPriceMinor()),
		CreatedAt: tsFromPgtz(row.CreatedAt),
	}}, nil
}

func (s *CourseServer) UpdateCourse(ctx context.Context, req *coursesv1.UpdateCourseRequest) (*coursesv1.UpdateCourseResponse, error) {
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
	in := courses.CreateCourseRequest{
		Title: req.GetTitle(), Summary: req.GetSummary(), Language: req.GetLanguage(),
		Level: req.GetLevel(), PriceMinor: req.GetPriceMinor(), HsnSac: req.GetHsnSac(),
		TaxRateBps: req.GetTaxRateBps(),
	}
	row, err := s.svc.Update(ctx, c.TenantID, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &coursesv1.UpdateCourseResponse{Course: &coursesv1.Course{
		Id: utils.UUIDFromPg(row.ID), Title: row.Title, Slug: row.Slug,
		Status: string(row.Status), IsPublished: row.Status == db.PublishStatusPublished,
	}}, nil
}

func (s *CourseServer) SetCoursePublished(ctx context.Context, req *coursesv1.SetCoursePublishedRequest) (*coursesv1.SetCoursePublishedResponse, error) {
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
	if err := s.svc.SetPublished(ctx, id, req.GetPublished()); err != nil {
		return nil, toStatus(err)
	}
	return &coursesv1.SetCoursePublishedResponse{}, nil
}

func (s *CourseServer) SetCoursePrice(ctx context.Context, req *coursesv1.SetCoursePriceRequest) (*coursesv1.SetCoursePriceResponse, error) {
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
	if req.GetAmountMinor() < 0 {
		return nil, status.Error(codes.InvalidArgument, "amount_minor must be >= 0")
	}
	if err := s.svc.SetPrice(ctx, c.TenantID, id, req.GetAmountMinor(), req.GetTaxRateBps()); err != nil {
		return nil, toStatus(err)
	}
	return &coursesv1.SetCoursePriceResponse{}, nil
}

func (s *CourseServer) ApproveCourse(ctx context.Context, req *coursesv1.ApproveCourseRequest) (*coursesv1.ApproveCourseResponse, error) {
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
	row, err := s.svc.Approve(ctx, id, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &coursesv1.ApproveCourseResponse{Course: &coursesv1.Course{
		Id: utils.UUIDFromPg(row.ID), Status: string(row.Status), ApprovalStatus: row.ApprovalStatus,
		IsPublished: row.Status == db.PublishStatusPublished,
	}}, nil
}

func (s *CourseServer) RejectCourse(ctx context.Context, req *coursesv1.RejectCourseRequest) (*coursesv1.RejectCourseResponse, error) {
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
	row, err := s.svc.Reject(ctx, id, req.GetReason())
	if err != nil {
		return nil, toStatus(err)
	}
	return &coursesv1.RejectCourseResponse{Course: &coursesv1.Course{
		Id: utils.UUIDFromPg(row.ID), ApprovalStatus: row.ApprovalStatus,
	}}, nil
}

func (s *CourseServer) DeleteCourse(ctx context.Context, req *coursesv1.DeleteCourseRequest) (*coursesv1.DeleteCourseResponse, error) {
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
	return &coursesv1.DeleteCourseResponse{}, nil
}
