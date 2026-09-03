package grpcserver

import (
	"context"

	chaptersv1 "live-platform/gen/proto/live/chapters/v1"
	subjectsv1 "live-platform/gen/proto/live/subjects/v1"
	topicsv1 "live-platform/gen/proto/live/topics/v1"
	"live-platform/internal/chapters"
	"live-platform/internal/subjects"
	"live-platform/internal/topics"
	"live-platform/internal/utils"
)

// ───────────────────────────────────────────────────────────── subjects

type SubjectServer struct {
	subjectsv1.UnimplementedSubjectServiceServer
	svc *subjects.Service
}

func NewSubjectServer(svc *subjects.Service) *SubjectServer { return &SubjectServer{svc: svc} }

func subjectMsg(id, name string, code, icon pgText, examCat pgUUID, order int32) *subjectsv1.Subject {
	return &subjectsv1.Subject{
		Id: id, Name: name, Code: utils.TextFromPg(code), IconUrl: utils.TextFromPg(icon),
		ExamCategoryId: utils.UUIDFromPg(examCat), DisplayOrder: order,
	}
}

func (s *SubjectServer) ListSubjects(ctx context.Context, _ *subjectsv1.ListSubjectsRequest) (*subjectsv1.ListSubjectsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListForTenant(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &subjectsv1.ListSubjectsResponse{}
	for _, r := range rows {
		out.Subjects = append(out.Subjects, subjectMsg(utils.UUIDFromPg(r.ID), r.Name, r.Code, r.IconUrl, r.ExamCategoryID, r.DisplayOrder))
	}
	return out, nil
}

func (s *SubjectServer) GetSubject(ctx context.Context, req *subjectsv1.GetSubjectRequest) (*subjectsv1.GetSubjectResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &subjectsv1.GetSubjectResponse{Subject: subjectMsg(utils.UUIDFromPg(r.ID), r.Name, r.Code, r.IconUrl, r.ExamCategoryID, r.DisplayOrder)}, nil
}

func subjectInput(in *subjectsv1.SubjectInput) (subjects.UpsertSubjectRequest, error) {
	r := subjects.UpsertSubjectRequest{
		Name: in.GetName(), Code: in.GetCode(), IconURL: in.GetIconUrl(), DisplayOrder: in.GetDisplayOrder(),
	}
	ec, err := optUUID(in.GetExamCategoryId(), "exam_category_id")
	if err != nil {
		return r, err
	}
	r.ExamCategoryID = ec
	return r, nil
}

func (s *SubjectServer) CreateSubject(ctx context.Context, req *subjectsv1.CreateSubjectRequest) (*subjectsv1.CreateSubjectResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	in, err := subjectInput(req.GetSubject())
	if err != nil {
		return nil, err
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	r, err := s.svc.Create(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &subjectsv1.CreateSubjectResponse{Subject: subjectMsg(utils.UUIDFromPg(r.ID), r.Name, r.Code, r.IconUrl, r.ExamCategoryID, r.DisplayOrder)}, nil
}

func (s *SubjectServer) UpdateSubject(ctx context.Context, req *subjectsv1.UpdateSubjectRequest) (*subjectsv1.UpdateSubjectResponse, error) {
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
	in, err := subjectInput(req.GetSubject())
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Update(ctx, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &subjectsv1.UpdateSubjectResponse{Subject: subjectMsg(utils.UUIDFromPg(r.ID), r.Name, r.Code, r.IconUrl, r.ExamCategoryID, r.DisplayOrder)}, nil
}

func (s *SubjectServer) DeleteSubject(ctx context.Context, req *subjectsv1.DeleteSubjectRequest) (*subjectsv1.DeleteSubjectResponse, error) {
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
	return &subjectsv1.DeleteSubjectResponse{}, nil
}

// ───────────────────────────────────────────────────────────── chapters

type ChapterServer struct {
	chaptersv1.UnimplementedChapterServiceServer
	svc *chapters.Service
}

func NewChapterServer(svc *chapters.Service) *ChapterServer { return &ChapterServer{svc: svc} }

func chapterMsg(id, subjectID, name string, desc pgText, order int32, free bool) *chaptersv1.Chapter {
	return &chaptersv1.Chapter{
		Id: id, SubjectId: subjectID, Name: name, Description: utils.TextFromPg(desc),
		DisplayOrder: order, IsFree: free,
	}
}

func (s *ChapterServer) ListChaptersBySubject(ctx context.Context, req *chaptersv1.ListChaptersBySubjectRequest) (*chaptersv1.ListChaptersBySubjectResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	subjectID, err := parseUUID(req.GetSubjectId(), "subject_id")
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListBySubject(ctx, c.TenantID, subjectID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &chaptersv1.ListChaptersBySubjectResponse{}
	for _, r := range rows {
		out.Chapters = append(out.Chapters, chapterMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.SubjectID), r.Name, r.Description, r.DisplayOrder, r.IsFree))
	}
	return out, nil
}

func (s *ChapterServer) GetChapter(ctx context.Context, req *chaptersv1.GetChapterRequest) (*chaptersv1.GetChapterResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &chaptersv1.GetChapterResponse{Chapter: chapterMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.SubjectID), r.Name, r.Description, r.DisplayOrder, r.IsFree)}, nil
}

func chapterInput(in *chaptersv1.ChapterInput) (chapters.UpsertChapterRequest, error) {
	r := chapters.UpsertChapterRequest{
		Name: in.GetName(), Description: in.GetDescription(), DisplayOrder: in.GetDisplayOrder(), IsFree: in.GetIsFree(),
	}
	sid, err := parseUUID(in.GetSubjectId(), "subject_id")
	if err != nil {
		return r, err
	}
	r.SubjectID = sid
	return r, nil
}

func (s *ChapterServer) CreateChapter(ctx context.Context, req *chaptersv1.CreateChapterRequest) (*chaptersv1.CreateChapterResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	in, err := chapterInput(req.GetChapter())
	if err != nil {
		return nil, err
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	r, err := s.svc.Create(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &chaptersv1.CreateChapterResponse{Chapter: chapterMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.SubjectID), r.Name, r.Description, r.DisplayOrder, r.IsFree)}, nil
}

func (s *ChapterServer) UpdateChapter(ctx context.Context, req *chaptersv1.UpdateChapterRequest) (*chaptersv1.UpdateChapterResponse, error) {
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
	in, err := chapterInput(req.GetChapter())
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Update(ctx, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &chaptersv1.UpdateChapterResponse{Chapter: chapterMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.SubjectID), r.Name, r.Description, r.DisplayOrder, r.IsFree)}, nil
}

func (s *ChapterServer) DeleteChapter(ctx context.Context, req *chaptersv1.DeleteChapterRequest) (*chaptersv1.DeleteChapterResponse, error) {
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
	return &chaptersv1.DeleteChapterResponse{}, nil
}

// ───────────────────────────────────────────────────────────── topics

type TopicServer struct {
	topicsv1.UnimplementedTopicServiceServer
	svc *topics.Service
}

func NewTopicServer(svc *topics.Service) *TopicServer { return &TopicServer{svc: svc} }

func topicMsg(id, chapterID, name string, desc pgText, order int32, free bool) *topicsv1.Topic {
	return &topicsv1.Topic{
		Id: id, ChapterId: chapterID, Name: name, Description: utils.TextFromPg(desc),
		DisplayOrder: order, IsFree: free,
	}
}

func (s *TopicServer) ListTopicsByChapter(ctx context.Context, req *topicsv1.ListTopicsByChapterRequest) (*topicsv1.ListTopicsByChapterResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	chapterID, err := parseUUID(req.GetChapterId(), "chapter_id")
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListByChapter(ctx, c.TenantID, chapterID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &topicsv1.ListTopicsByChapterResponse{}
	for _, r := range rows {
		out.Topics = append(out.Topics, topicMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.ChapterID), r.Name, r.Description, r.DisplayOrder, r.IsFree))
	}
	return out, nil
}

func (s *TopicServer) GetTopic(ctx context.Context, req *topicsv1.GetTopicRequest) (*topicsv1.GetTopicResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &topicsv1.GetTopicResponse{Topic: topicMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.ChapterID), r.Name, r.Description, r.DisplayOrder, r.IsFree)}, nil
}

func topicInput(in *topicsv1.TopicInput) (topics.UpsertTopicRequest, error) {
	r := topics.UpsertTopicRequest{
		Name: in.GetName(), Description: in.GetDescription(), DisplayOrder: in.GetDisplayOrder(), IsFree: in.GetIsFree(),
	}
	cid, err := parseUUID(in.GetChapterId(), "chapter_id")
	if err != nil {
		return r, err
	}
	r.ChapterID = cid
	return r, nil
}

func (s *TopicServer) CreateTopic(ctx context.Context, req *topicsv1.CreateTopicRequest) (*topicsv1.CreateTopicResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	in, err := topicInput(req.GetTopic())
	if err != nil {
		return nil, err
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	r, err := s.svc.Create(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &topicsv1.CreateTopicResponse{Topic: topicMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.ChapterID), r.Name, r.Description, r.DisplayOrder, r.IsFree)}, nil
}

func (s *TopicServer) UpdateTopic(ctx context.Context, req *topicsv1.UpdateTopicRequest) (*topicsv1.UpdateTopicResponse, error) {
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
	in, err := topicInput(req.GetTopic())
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Update(ctx, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &topicsv1.UpdateTopicResponse{Topic: topicMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.ChapterID), r.Name, r.Description, r.DisplayOrder, r.IsFree)}, nil
}

func (s *TopicServer) DeleteTopic(ctx context.Context, req *topicsv1.DeleteTopicRequest) (*topicsv1.DeleteTopicResponse, error) {
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
	return &topicsv1.DeleteTopicResponse{}, nil
}
