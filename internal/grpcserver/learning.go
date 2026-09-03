package grpcserver

import (
	"context"

	bookmarksv1 "live-platform/gen/proto/live/bookmarks/v1"
	enrollmentsv1 "live-platform/gen/proto/live/enrollments/v1"
	"live-platform/internal/bookmarks"
	"live-platform/internal/enrollments"
	"live-platform/internal/utils"
)

// ─────────────────────────────────────────────────────────── enrollments

type EnrollmentServer struct {
	enrollmentsv1.UnimplementedEnrollmentServiceServer
	svc *enrollments.Service
}

func NewEnrollmentServer(svc *enrollments.Service) *EnrollmentServer {
	return &EnrollmentServer{svc: svc}
}

func (s *EnrollmentServer) Enroll(ctx context.Context, req *enrollmentsv1.EnrollRequest) (*enrollmentsv1.EnrollResponse, error) {
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
	batchID, err := optUUID(req.GetBatchId(), "batch_id")
	if err != nil {
		return nil, err
	}
	in := enrollments.EnrollRequest{CourseID: courseID, BatchID: batchID}
	if err := validate(&in); err != nil {
		return nil, err
	}
	row, err := s.svc.Enroll(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &enrollmentsv1.EnrollResponse{Enrollment: &enrollmentsv1.Enrollment{
		Id: utils.UUIDFromPg(row.ID), CourseId: utils.UUIDFromPg(row.CourseID),
		BatchId: utils.UUIDFromPg(row.BatchID), Status: string(row.Status),
		ProgressBps: row.ProgressBps, StartedAt: tsFromPgtz(row.StartedAt),
	}}, nil
}

func (s *EnrollmentServer) GetEnrollment(ctx context.Context, req *enrollmentsv1.GetEnrollmentRequest) (*enrollmentsv1.GetEnrollmentResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	row, err := s.svc.Get(ctx, c.TenantID, c.UserID, courseID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &enrollmentsv1.GetEnrollmentResponse{Enrollment: &enrollmentsv1.Enrollment{
		Id: utils.UUIDFromPg(row.ID), CourseId: utils.UUIDFromPg(row.CourseID),
		BatchId: utils.UUIDFromPg(row.BatchID), Status: string(row.Status),
		ProgressBps: row.ProgressBps, StartedAt: tsFromPgtz(row.StartedAt), CompletedAt: tsFromPgtz(row.CompletedAt),
	}}, nil
}

func (s *EnrollmentServer) ListMyEnrollments(ctx context.Context, _ *enrollmentsv1.ListMyEnrollmentsRequest) (*enrollmentsv1.ListMyEnrollmentsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListMine(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &enrollmentsv1.ListMyEnrollmentsResponse{}
	for _, r := range rows {
		out.Enrollments = append(out.Enrollments, &enrollmentsv1.Enrollment{
			Id: utils.UUIDFromPg(r.ID), CourseId: utils.UUIDFromPg(r.CourseID), BatchId: utils.UUIDFromPg(r.BatchID),
			Status: string(r.Status), ProgressBps: r.ProgressBps, StartedAt: tsFromPgtz(r.StartedAt),
			CompletedAt: tsFromPgtz(r.CompletedAt), CourseTitle: r.Title, CourseSlug: r.Slug,
			ThumbnailUrl: utils.TextFromPg(r.ThumbnailUrl),
		})
	}
	return out, nil
}

func (s *EnrollmentServer) ListCourseRoster(ctx context.Context, req *enrollmentsv1.ListCourseRosterRequest) (*enrollmentsv1.ListCourseRosterResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListRoster(ctx, c.TenantID, courseID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &enrollmentsv1.ListCourseRosterResponse{}
	for _, r := range rows {
		out.Entries = append(out.Entries, &enrollmentsv1.RosterEntry{
			EnrollmentId: utils.UUIDFromPg(r.ID), UserId: utils.UUIDFromPg(r.UserID), BatchId: utils.UUIDFromPg(r.BatchID),
			Status: string(r.Status), ProgressBps: r.ProgressBps, FullName: utils.TextFromPg(r.FullName),
			Email: utils.TextFromPg(r.Email), Phone: utils.TextFromPg(r.Phone),
		})
	}
	return out, nil
}

func (s *EnrollmentServer) UpdateProgress(ctx context.Context, req *enrollmentsv1.UpdateProgressRequest) (*enrollmentsv1.UpdateProgressResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.UpdateProgress(ctx, c.TenantID, c.UserID, courseID, req.GetProgressPercent()); err != nil {
		return nil, toStatus(err)
	}
	return &enrollmentsv1.UpdateProgressResponse{}, nil
}

func (s *EnrollmentServer) CancelEnrollment(ctx context.Context, req *enrollmentsv1.CancelEnrollmentRequest) (*enrollmentsv1.CancelEnrollmentResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Cancel(ctx, c.TenantID, c.UserID, courseID); err != nil {
		return nil, toStatus(err)
	}
	return &enrollmentsv1.CancelEnrollmentResponse{}, nil
}

// ─────────────────────────────────────────────────────────── bookmarks

type BookmarkServer struct {
	bookmarksv1.UnimplementedBookmarkServiceServer
	svc *bookmarks.Service
}

func NewBookmarkServer(svc *bookmarks.Service) *BookmarkServer { return &BookmarkServer{svc: svc} }

func bookmarkMsg(id, lessonID string, pos int32, note pgText, created pgTs) *bookmarksv1.Bookmark {
	return &bookmarksv1.Bookmark{
		Id: id, LessonId: lessonID, PositionSec: pos, Note: utils.TextFromPg(note), CreatedAt: tsFromPgtz(created),
	}
}

func (s *BookmarkServer) ListMyBookmarks(ctx context.Context, _ *bookmarksv1.ListMyBookmarksRequest) (*bookmarksv1.ListMyBookmarksResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListMine(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &bookmarksv1.ListMyBookmarksResponse{}
	for _, r := range rows {
		out.Bookmarks = append(out.Bookmarks, bookmarkMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.LessonID), r.PositionSec, r.Note, r.CreatedAt))
	}
	return out, nil
}

func (s *BookmarkServer) CreateBookmark(ctx context.Context, req *bookmarksv1.CreateBookmarkRequest) (*bookmarksv1.CreateBookmarkResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	lessonID, err := parseUUID(req.GetLessonId(), "lesson_id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Create(ctx, c.TenantID, c.UserID, bookmarks.CreateRequest{
		LessonID: &lessonID, PositionSec: req.GetPositionSec(), Note: req.GetNote(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &bookmarksv1.CreateBookmarkResponse{Bookmark: bookmarkMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.LessonID), r.PositionSec, r.Note, r.CreatedAt)}, nil
}

func (s *BookmarkServer) DeleteBookmark(ctx context.Context, req *bookmarksv1.DeleteBookmarkRequest) (*bookmarksv1.DeleteBookmarkResponse, error) {
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
	return &bookmarksv1.DeleteBookmarkResponse{}, nil
}
