package grpcserver

import (
	"context"

	lecturesv1 "live-platform/gen/proto/live/lectures/v1"
	"live-platform/internal/lectures"
)

type LectureServer struct {
	lecturesv1.UnimplementedLectureServiceServer
	svc *lectures.Service
}

func NewLectureServer(svc *lectures.Service) *LectureServer { return &LectureServer{svc: svc} }

func lectureMsg(l lectures.Lesson) *lecturesv1.Lecture {
	return &lecturesv1.Lecture{
		Id: l.ID, CourseId: l.CourseID, CourseTitle: l.CourseTitle, SectionId: l.SectionID,
		Title: l.Title, ContentKind: l.ContentKind, VideoUrl: l.VideoURL, HlsUrl: l.HlsURL,
		DocumentUrl: l.DocumentURL, LinkUrl: l.LinkURL, DurationSeconds: l.DurationSec,
		IsPreview: l.IsPreview, IsFree: l.IsFree, IsPublished: l.IsPublished,
		DisplayOrder: l.DisplayOrder, AvailableAt: tsFromTime(l.AvailableAt),
	}
}

func sectionMsg(s lectures.Section) *lecturesv1.Section {
	return &lecturesv1.Section{
		Id: s.ID, CourseId: s.CourseID, Title: s.Title,
		DisplayOrder: s.DisplayOrder, DripAfterDays: s.DripAfterDays,
	}
}

func (s *LectureServer) ListLectures(ctx context.Context, req *lecturesv1.ListLecturesRequest) (*lecturesv1.ListLecturesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	courseID, err := optUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListForTenant(ctx, c.TenantID, courseID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &lecturesv1.ListLecturesResponse{}
	for _, l := range rows {
		out.Lectures = append(out.Lectures, lectureMsg(l))
	}
	return out, nil
}

func (s *LectureServer) GetLecture(ctx context.Context, req *lecturesv1.GetLectureRequest) (*lecturesv1.GetLectureResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	l, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &lecturesv1.GetLectureResponse{Lecture: lectureMsg(l)}, nil
}

func lectureInput(in *lecturesv1.LectureInput) (lectures.CreateLectureRequest, error) {
	r := lectures.CreateLectureRequest{
		Title: in.GetTitle(), Description: in.GetDescription(), VideoURL: in.GetVideoUrl(),
		DocumentURL: in.GetDocumentUrl(), LinkURL: in.GetLinkUrl(), ThumbnailURL: in.GetThumbnailUrl(),
		DurationSec: in.GetDurationSeconds(), IsFree: in.GetIsFree(), IsPublished: in.GetIsPublished(),
		DisplayOrder: in.GetDisplayOrder(),
	}
	cid, err := optUUID(in.GetCourseId(), "course_id")
	if err != nil {
		return r, err
	}
	sid, err := optUUID(in.GetSectionId(), "section_id")
	if err != nil {
		return r, err
	}
	r.CourseID, r.SectionID = cid, sid
	return r, nil
}

func (s *LectureServer) CreateLecture(ctx context.Context, req *lecturesv1.CreateLectureRequest) (*lecturesv1.CreateLectureResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	in, err := lectureInput(req.GetLecture())
	if err != nil {
		return nil, err
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	l, err := s.svc.Create(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &lecturesv1.CreateLectureResponse{Lecture: lectureMsg(l)}, nil
}

func (s *LectureServer) UpdateLecture(ctx context.Context, req *lecturesv1.UpdateLectureRequest) (*lecturesv1.UpdateLectureResponse, error) {
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
	in, err := lectureInput(req.GetLecture())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Update(ctx, id, in); err != nil {
		return nil, toStatus(err)
	}
	return &lecturesv1.UpdateLectureResponse{}, nil
}

func (s *LectureServer) DeleteLecture(ctx context.Context, req *lecturesv1.DeleteLectureRequest) (*lecturesv1.DeleteLectureResponse, error) {
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
	if err := s.svc.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &lecturesv1.DeleteLectureResponse{}, nil
}

func (s *LectureServer) ListSections(ctx context.Context, req *lecturesv1.ListSectionsRequest) (*lecturesv1.ListSectionsResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListSections(ctx, courseID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &lecturesv1.ListSectionsResponse{}
	for _, sec := range rows {
		out.Sections = append(out.Sections, sectionMsg(sec))
	}
	return out, nil
}

func sectionInput(in *lecturesv1.SectionInput) (lectures.SectionRequest, error) {
	r := lectures.SectionRequest{
		Title: in.GetTitle(), DisplayOrder: in.GetDisplayOrder(), DripAfterDays: in.GetDripAfterDays(),
	}
	cid, err := optUUID(in.GetCourseId(), "course_id")
	if err != nil {
		return r, err
	}
	r.CourseID = cid
	return r, nil
}

func (s *LectureServer) CreateSection(ctx context.Context, req *lecturesv1.CreateSectionRequest) (*lecturesv1.CreateSectionResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	in, err := sectionInput(req.GetSection())
	if err != nil {
		return nil, err
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	sec, err := s.svc.CreateSection(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &lecturesv1.CreateSectionResponse{Section: sectionMsg(sec)}, nil
}

func (s *LectureServer) UpdateSection(ctx context.Context, req *lecturesv1.UpdateSectionRequest) (*lecturesv1.UpdateSectionResponse, error) {
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
	in, err := sectionInput(req.GetSection())
	if err != nil {
		return nil, err
	}
	if err := s.svc.UpdateSection(ctx, id, in); err != nil {
		return nil, toStatus(err)
	}
	return &lecturesv1.UpdateSectionResponse{}, nil
}

func (s *LectureServer) DeleteSection(ctx context.Context, req *lecturesv1.DeleteSectionRequest) (*lecturesv1.DeleteSectionResponse, error) {
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
	if err := s.svc.DeleteSection(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &lecturesv1.DeleteSectionResponse{}, nil
}

func (s *LectureServer) RecordWatch(ctx context.Context, req *lecturesv1.RecordWatchRequest) (*lecturesv1.RecordWatchResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	lessonID, err := parseUUID(req.GetLessonId(), "lesson_id")
	if err != nil {
		return nil, err
	}
	in := lectures.RecordWatchRequest{
		LessonID: &lessonID, WatchedSeconds: req.GetWatchedSeconds(),
		PositionSec: req.GetPositionSeconds(), Completed: req.GetCompleted(),
	}
	if err := s.svc.RecordWatch(ctx, c.TenantID, c.UserID, in); err != nil {
		return nil, toStatus(err)
	}
	return &lecturesv1.RecordWatchResponse{}, nil
}
