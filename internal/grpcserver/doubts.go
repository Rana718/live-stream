package grpcserver

import (
	"context"

	doubtsv1 "live-platform/gen/proto/live/doubts/v1"
	"live-platform/internal/database/db"
	"live-platform/internal/doubts"
	"live-platform/internal/utils"
)

type DoubtServer struct {
	doubtsv1.UnimplementedDoubtServiceServer
	svc *doubts.Service
}

func NewDoubtServer(svc *doubts.Service) *DoubtServer { return &DoubtServer{svc: svc} }

func answerMsg(a db.AddDoubtAnswerRow) *doubtsv1.DoubtAnswer {
	return &doubtsv1.DoubtAnswer{
		Id: utils.UUIDFromPg(a.ID), AnswerText: a.AnswerText, AnswerType: a.AnswerType,
		AnsweredBy: utils.UUIDFromPg(a.AnsweredBy), IsAccepted: a.IsAccepted, CreatedAt: tsFromPgtz(a.CreatedAt),
	}
}

func (s *DoubtServer) AskDoubt(ctx context.Context, req *doubtsv1.AskDoubtRequest) (*doubtsv1.AskDoubtResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	lessonID, err := optUUID(req.GetLessonId(), "lesson_id")
	if err != nil {
		return nil, err
	}
	chapterID, err := optUUID(req.GetChapterId(), "chapter_id")
	if err != nil {
		return nil, err
	}
	topicID, err := optUUID(req.GetTopicId(), "topic_id")
	if err != nil {
		return nil, err
	}
	in := doubts.AskDoubtRequest{
		LessonID: lessonID, ChapterID: chapterID, TopicID: topicID,
		QuestionText: req.GetQuestionText(), InputType: req.GetInputType(),
		AttachmentURL: req.GetAttachmentUrl(), Language: req.GetLanguage(), UseAI: req.GetUseAi(),
	}
	doubt, aiAns, err := s.svc.Ask(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &doubtsv1.AskDoubtResponse{Doubt: &doubtsv1.Doubt{
		Id: utils.UUIDFromPg(doubt.ID), UserId: utils.UUIDFromPg(doubt.UserID),
		QuestionText: doubt.QuestionText, Status: string(doubt.Status), CreatedAt: tsFromPgtz(doubt.CreatedAt),
	}}
	if aiAns != nil {
		resp.AiAnswer = answerMsg(*aiAns)
	}
	return resp, nil
}

func (s *DoubtServer) GetDoubt(ctx context.Context, req *doubtsv1.GetDoubtRequest) (*doubtsv1.GetDoubtResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	d, answers, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &doubtsv1.GetDoubtResponse{Doubt: &doubtsv1.Doubt{
		Id: utils.UUIDFromPg(d.ID), UserId: utils.UUIDFromPg(d.UserID),
		QuestionText: d.QuestionText, Status: string(d.Status), CreatedAt: tsFromPgtz(d.CreatedAt),
	}}
	for _, a := range answers {
		out.Answers = append(out.Answers, &doubtsv1.DoubtAnswer{
			Id: utils.UUIDFromPg(a.ID), AnswerText: a.AnswerText, AnswerType: a.AnswerType,
			AnsweredBy: utils.UUIDFromPg(a.AnsweredBy), IsAccepted: a.IsAccepted,
			ModelName: utils.TextFromPg(a.ModelName), CreatedAt: tsFromPgtz(a.CreatedAt),
		})
	}
	return out, nil
}

func (s *DoubtServer) ListMyDoubts(ctx context.Context, req *doubtsv1.ListMyDoubtsRequest) (*doubtsv1.ListMyDoubtsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListMine(ctx, c.TenantID, c.UserID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &doubtsv1.ListMyDoubtsResponse{}
	for _, d := range rows {
		out.Doubts = append(out.Doubts, &doubtsv1.Doubt{
			Id: utils.UUIDFromPg(d.ID), QuestionText: d.QuestionText, Status: string(d.Status), CreatedAt: tsFromPgtz(d.CreatedAt),
		})
	}
	return out, nil
}

func (s *DoubtServer) ListPendingDoubts(ctx context.Context, req *doubtsv1.ListPendingDoubtsRequest) (*doubtsv1.ListPendingDoubtsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListPending(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &doubtsv1.ListPendingDoubtsResponse{}
	for _, d := range rows {
		out.Doubts = append(out.Doubts, &doubtsv1.Doubt{
			Id: utils.UUIDFromPg(d.ID), UserId: utils.UUIDFromPg(d.UserID), QuestionText: d.QuestionText,
			FullName: utils.TextFromPg(d.FullName), CreatedAt: tsFromPgtz(d.CreatedAt),
		})
	}
	return out, nil
}

func (s *DoubtServer) ListAllDoubts(ctx context.Context, req *doubtsv1.ListAllDoubtsRequest) (*doubtsv1.ListAllDoubtsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListAllForTenant(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &doubtsv1.ListAllDoubtsResponse{}
	for _, d := range rows {
		out.Doubts = append(out.Doubts, &doubtsv1.Doubt{
			Id: utils.UUIDFromPg(d.ID), UserId: utils.UUIDFromPg(d.UserID), QuestionText: d.QuestionText,
			Status: string(d.Status), FullName: utils.TextFromPg(d.FullName), AnswersCount: d.AnswersCount,
			CreatedAt: tsFromPgtz(d.CreatedAt),
		})
	}
	return out, nil
}

func (s *DoubtServer) AnswerDoubt(ctx context.Context, req *doubtsv1.AnswerDoubtRequest) (*doubtsv1.AnswerDoubtResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	doubtID, err := parseUUID(req.GetDoubtId(), "doubt_id")
	if err != nil {
		return nil, err
	}
	in := doubts.InstructorAnswerRequest{DoubtID: doubtID, AnswerText: req.GetAnswerText()}
	if err := validate(&in); err != nil {
		return nil, err
	}
	a, err := s.svc.AnswerAsInstructor(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &doubtsv1.AnswerDoubtResponse{Answer: answerMsg(a)}, nil
}

func (s *DoubtServer) AcceptAnswer(ctx context.Context, req *doubtsv1.AcceptAnswerRequest) (*doubtsv1.AcceptAnswerResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetAnswerId(), "answer_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Accept(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &doubtsv1.AcceptAnswerResponse{}, nil
}

func (s *DoubtServer) SetDoubtStatus(ctx context.Context, req *doubtsv1.SetDoubtStatusRequest) (*doubtsv1.SetDoubtStatusResponse, error) {
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
	if err := s.svc.SetStatus(ctx, id, req.GetStatus()); err != nil {
		return nil, toStatus(err)
	}
	return &doubtsv1.SetDoubtStatusResponse{}, nil
}
