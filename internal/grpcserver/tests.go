package grpcserver

import (
	"context"

	testsv1 "live-platform/gen/proto/live/tests/v1"
	"live-platform/internal/tests"
	"live-platform/internal/utils"

	"github.com/google/uuid"
)

type TestServer struct {
	testsv1.UnimplementedTestServiceServer
	svc *tests.Service
}

func NewTestServer(svc *tests.Service) *TestServer { return &TestServer{svc: svc} }

func testViewMsg(v tests.TestView) *testsv1.Test {
	m := &testsv1.Test{
		Id: v.ID, Title: v.Title, Description: v.Description, Kind: v.Kind, DurationMinutes: v.DurationMinutes,
		TotalMarks: v.TotalMarks, NegativeMarking: v.NegativeMarking, IsPublished: v.IsPublished,
		Status: v.Status, QuestionCount: int32(v.QuestionCount),
	}
	for _, q := range v.Questions {
		qm := &testsv1.Question{
			Id: q.ID, QuestionText: q.QuestionText, QuestionType: q.QuestionType, ImageUrl: q.ImageURL, Marks: q.Marks,
		}
		for _, o := range q.Options {
			qm.Options = append(qm.Options, &testsv1.Option{Id: o.ID, OptionText: o.OptionText, ImageUrl: o.ImageURL})
		}
		m.Questions = append(m.Questions, qm)
	}
	return m
}

// attemptFromMap transcribes the tests.Service map[string]any attempt shape.
func attemptFromMap(m map[string]any) *testsv1.Attempt {
	return &testsv1.Attempt{
		AttemptId: asString(firstNonNil(m["attempt_id"], m["id"])), TestId: asString(m["test_id"]),
		AttemptNo: int32(asInt(m["attempt_no"])), Status: asString(m["status"]),
		Score: asFloat(m["score"]), MaxScore: asFloat(m["max_score"]),
		CorrectCount: int32(asInt(m["correct_count"])), WrongCount: int32(asInt(m["wrong_count"])),
		SkippedCount: int32(asInt(m["skipped_count"])), StartedAt: asTimestamp(m["started_at"]),
	}
}

func firstNonNil(vs ...any) any {
	for _, v := range vs {
		if v != nil {
			if s, ok := v.(string); ok && s == "" {
				continue
			}
			return v
		}
	}
	return nil
}

func (s *TestServer) ListTests(ctx context.Context, req *testsv1.ListTestsRequest) (*testsv1.ListTestsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := optUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListTests(ctx, c.TenantID, courseID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &testsv1.ListTestsResponse{}
	for _, r := range rows {
		out.Tests = append(out.Tests, &testsv1.TestListItem{
			Id: utils.UUIDFromPg(r.ID), Title: r.Title, Kind: string(r.Kind), DurationMin: r.DurationMin,
			TotalMarks: utils.NumericToFloat(r.TotalMarks), IsFree: r.IsFree, Status: string(r.Status),
		})
	}
	return out, nil
}

func (s *TestServer) GetTest(ctx context.Context, req *testsv1.GetTestRequest) (*testsv1.GetTestResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	v, err := s.svc.GetTest(ctx, id, req.GetWithQuestions())
	if err != nil {
		return nil, toStatus(err)
	}
	return &testsv1.GetTestResponse{Test: testViewMsg(v)}, nil
}

func (s *TestServer) CreateTest(ctx context.Context, req *testsv1.CreateTestRequest) (*testsv1.CreateTestResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	in := tests.CreateTestRequest{
		Title: req.GetTitle(), Description: req.GetDescription(), Kind: req.GetKind(),
		DurationMinutes: req.GetDurationMinutes(), TotalMarks: req.GetTotalMarks(), PassingMarks: req.GetPassingMarks(),
		NegativeMarking: req.GetNegativeMarking(), ShuffleQuestions: req.GetShuffleQuestions(),
	}
	if in.CourseID, err = optUUID(req.GetCourseId(), "course_id"); err != nil {
		return nil, err
	}
	if in.SubjectID, err = optUUID(req.GetSubjectId(), "subject_id"); err != nil {
		return nil, err
	}
	if in.ChapterID, err = optUUID(req.GetChapterId(), "chapter_id"); err != nil {
		return nil, err
	}
	if in.TopicID, err = optUUID(req.GetTopicId(), "topic_id"); err != nil {
		return nil, err
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	row, err := s.svc.CreateTest(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &testsv1.CreateTestResponse{Test: &testsv1.Test{
		Id: utils.UUIDFromPg(row.ID), Title: row.Title, Kind: string(row.Kind), Status: string(row.Status),
		TotalMarks: utils.NumericToFloat(row.TotalMarks),
	}}, nil
}

func (s *TestServer) UpdateTest(ctx context.Context, req *testsv1.UpdateTestRequest) (*testsv1.UpdateTestResponse, error) {
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
	in := tests.CreateTestRequest{
		Title: req.GetTitle(), Description: req.GetDescription(), Kind: req.GetKind(),
		DurationMinutes: req.GetDurationMinutes(), TotalMarks: req.GetTotalMarks(), NegativeMarking: req.GetNegativeMarking(),
	}
	if err := s.svc.UpdateTest(ctx, id, in); err != nil {
		return nil, toStatus(err)
	}
	return &testsv1.UpdateTestResponse{}, nil
}

func (s *TestServer) SetTestPublished(ctx context.Context, req *testsv1.SetTestPublishedRequest) (*testsv1.SetTestPublishedResponse, error) {
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
	return &testsv1.SetTestPublishedResponse{}, nil
}

func (s *TestServer) DeleteTest(ctx context.Context, req *testsv1.DeleteTestRequest) (*testsv1.DeleteTestResponse, error) {
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
	if err := s.svc.DeleteTest(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &testsv1.DeleteTestResponse{}, nil
}

func (s *TestServer) CreateQuestion(ctx context.Context, req *testsv1.CreateQuestionRequest) (*testsv1.CreateQuestionResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	testID, err := optUUID(req.GetTestId(), "test_id")
	if err != nil {
		return nil, err
	}
	subjectID, err := optUUID(req.GetSubjectId(), "subject_id")
	if err != nil {
		return nil, err
	}
	topicID, err := optUUID(req.GetTopicId(), "topic_id")
	if err != nil {
		return nil, err
	}
	in := tests.CreateQuestionRequest{
		TestID: testID, SubjectID: subjectID, TopicID: topicID, QuestionText: req.GetQuestionText(),
		QuestionType: req.GetQuestionType(), ImageURL: req.GetImageUrl(), Marks: req.GetMarks(),
		NegativeMarks: req.GetNegativeMarks(), Difficulty: req.GetDifficulty(), Explanation: req.GetExplanation(),
		DisplayOrder: req.GetDisplayOrder(),
	}
	if req.GetCorrectNumericalAnswer() != 0 {
		v := req.GetCorrectNumericalAnswer()
		in.CorrectNumericalAnswer = &v
	}
	if req.GetNumericTolerance() != 0 {
		v := req.GetNumericTolerance()
		in.NumericTolerance = &v
	}
	for _, o := range req.GetOptions() {
		in.Options = append(in.Options, tests.QuestionOptionRequest{
			OptionText: o.GetOptionText(), ImageURL: o.GetImageUrl(), IsCorrect: o.GetIsCorrect(), DisplayOrder: o.GetDisplayOrder(),
		})
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	m, err := s.svc.CreateQuestion(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &testsv1.CreateQuestionResponse{QuestionId: asString(firstNonNil(m["id"], m["question_id"]))}, nil
}

func (s *TestServer) DeleteQuestion(ctx context.Context, req *testsv1.DeleteQuestionRequest) (*testsv1.DeleteQuestionResponse, error) {
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
	if err := s.svc.DeleteQuestion(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &testsv1.DeleteQuestionResponse{}, nil
}

func (s *TestServer) StartAttempt(ctx context.Context, req *testsv1.StartAttemptRequest) (*testsv1.StartAttemptResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	testID, err := parseUUID(req.GetTestId(), "test_id")
	if err != nil {
		return nil, err
	}
	m, err := s.svc.StartAttempt(ctx, c.TenantID, c.UserID, testID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &testsv1.StartAttemptResponse{Attempt: attemptFromMap(m)}, nil
}

func (s *TestServer) SubmitAnswer(ctx context.Context, req *testsv1.SubmitAnswerRequest) (*testsv1.SubmitAnswerResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	attemptID, err := parseUUID(req.GetAttemptId(), "attempt_id")
	if err != nil {
		return nil, err
	}
	questionID, err := parseUUID(req.GetQuestionId(), "question_id")
	if err != nil {
		return nil, err
	}
	in := tests.SubmitAnswerRequest{
		AttemptID: attemptID, QuestionID: questionID, SubjectiveAnswer: req.GetSubjectiveAnswer(),
		TimeTakenSeconds: req.GetTimeTakenSeconds(),
	}
	if req.GetSelectedOptionId() != "" {
		oid, perr := uuid.Parse(req.GetSelectedOptionId())
		if perr != nil {
			return nil, invalidArg("selected_option_id must be a uuid")
		}
		in.SelectedOptionID = &oid
	}
	if req.GetNumericalAnswer() != 0 {
		v := req.GetNumericalAnswer()
		in.NumericalAnswer = &v
	}
	if err := s.svc.SubmitAnswer(ctx, c.TenantID, c.UserID, in); err != nil {
		return nil, toStatus(err)
	}
	return &testsv1.SubmitAnswerResponse{}, nil
}

func (s *TestServer) SubmitAttempt(ctx context.Context, req *testsv1.SubmitAttemptRequest) (*testsv1.SubmitAttemptResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	attemptID, err := parseUUID(req.GetAttemptId(), "attempt_id")
	if err != nil {
		return nil, err
	}
	m, err := s.svc.SubmitAttempt(ctx, c.UserID, attemptID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &testsv1.SubmitAttemptResponse{Attempt: attemptFromMap(m)}, nil
}

func (s *TestServer) GetAttempt(ctx context.Context, req *testsv1.GetAttemptRequest) (*testsv1.GetAttemptResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	attemptID, err := parseUUID(req.GetAttemptId(), "attempt_id")
	if err != nil {
		return nil, err
	}
	m, err := s.svc.GetAttempt(ctx, attemptID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &testsv1.GetAttemptResponse{Attempt: attemptFromMap(m)}
	if raw, ok := m["answers"].([]map[string]any); ok {
		for _, a := range raw {
			ans := &testsv1.AttemptAnswer{
				QuestionId: asString(a["question_id"]), NumericalAnswer: asFloat(a["numerical_answer"]),
				SubjectiveAnswer: asString(a["subjective_answer"]), IsCorrect: asBool(a["is_correct"]), Marks: asFloat(a["marks"]),
			}
			if sel, ok := a["selected_option_ids"].([]string); ok {
				ans.SelectedOptionIds = sel
			}
			out.Answers = append(out.Answers, ans)
		}
	}
	return out, nil
}

func (s *TestServer) ListMyAttempts(ctx context.Context, req *testsv1.ListMyAttemptsRequest) (*testsv1.ListMyAttemptsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListMyAttempts(ctx, c.TenantID, c.UserID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &testsv1.ListMyAttemptsResponse{}
	for _, m := range rows {
		out.Attempts = append(out.Attempts, attemptFromMap(m))
	}
	return out, nil
}
