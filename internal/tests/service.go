// Package tests — schema-v2. Questions live in a reusable question_bank;
// tests are assembled from it via test_questions. Attempts carry attempt_no;
// responses are append-only (test_responses) and the latest row per question
// is authoritative. Scoring: SubmitAnswer scores each response as it lands;
// SubmitAttempt aggregates via ScoreAttemptResponses and finalises.
package tests

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, q: db.New(pool)} }

// ── enum mapping between the frontend vocab and question_kind/test_kind ──

func toQuestionKind(s string) db.QuestionKind {
	switch s {
	case "mcq", "mcq_single":
		return db.QuestionKind("mcq_single")
	case "mcq_multi":
		return db.QuestionKind("mcq_multi")
	case "numerical", "numeric":
		return db.QuestionKind("numeric")
	case "match":
		return db.QuestionKind("match")
	default:
		return db.QuestionKind("subjective")
	}
}

func fromQuestionKind(k db.QuestionKind) string {
	switch string(k) {
	case "mcq_single", "mcq_multi", "match":
		return "mcq"
	case "numeric":
		return "numerical"
	default:
		return "subjective"
	}
}

func toTestKind(s string) db.TestKind {
	switch s {
	case "dpp", "chapter_test", "subject_test", "mock", "pyq", "live_quiz":
		return db.TestKind(s)
	case "chapter":
		return db.TestKind("chapter_test")
	case "subject":
		return db.TestKind("subject_test")
	default:
		return db.TestKind("chapter_test")
	}
}

func stemText(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(b, &m) == nil {
		if v, ok := m["text"].(string); ok {
			return v
		}
	}
	return string(b)
}

func stemJSON(text string) []byte {
	b, _ := json.Marshal(map[string]string{"text": text})
	return b
}

func nnum(f float64) pgtype.Numeric {
	if f == 0 {
		return pgtype.Numeric{}
	}
	return utils.NumericFromFloat(f)
}
func ntext(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// ── tests ───────────────────────────────────────────────────────────

type CreateTestRequest struct {
	CourseID         *uuid.UUID `json:"course_id"`
	SubjectID        *uuid.UUID `json:"subject_id"`
	ChapterID        *uuid.UUID `json:"chapter_id"`
	TopicID          *uuid.UUID `json:"topic_id"`
	ExamCategoryID   *uuid.UUID `json:"exam_category_id"`
	Title            string     `json:"title" validate:"required,min=3"`
	Description      string     `json:"description"`
	TestType         string     `json:"test_type"`
	Kind             string     `json:"kind"`
	ExamYear         int32      `json:"exam_year"`
	DurationMinutes  int32      `json:"duration_minutes"`
	DurationMin      int32      `json:"duration_min"`
	TotalMarks       float64    `json:"total_marks"`
	PassingMarks     float64    `json:"passing_marks"`
	NegativeMarking  bool       `json:"negative_marking"`
	ShuffleQuestions bool       `json:"shuffle_questions"`
	Language         string     `json:"language"`
	IsFree           bool       `json:"is_free"`
	IsPublished      bool       `json:"is_published"`
}

func (r CreateTestRequest) kind() string {
	if r.Kind != "" {
		return r.Kind
	}
	return r.TestType
}
func (r CreateTestRequest) duration() int32 {
	if r.DurationMin > 0 {
		return r.DurationMin
	}
	return r.DurationMinutes
}

func statusFor(pub bool) db.PublishStatus {
	if pub {
		return db.PublishStatusPublished
	}
	return db.PublishStatusDraft
}

func (s *Service) CreateTest(ctx context.Context, tenantID, creator uuid.UUID, req CreateTestRequest) (db.CreateTestRow, error) {
	return s.q.CreateTest(ctx, db.CreateTestParams{
		TenantID:         utils.UUIDToPg(tenantID),
		Title:            req.Title,
		CourseID:         utils.UUIDPtrToPg(req.CourseID),
		SubjectID:        utils.UUIDPtrToPg(req.SubjectID),
		ChapterID:        utils.UUIDPtrToPg(req.ChapterID),
		TopicID:          utils.UUIDPtrToPg(req.TopicID),
		ExamCategoryID:   utils.UUIDPtrToPg(req.ExamCategoryID),
		Description:      ntext(req.Description),
		Kind:             db.NullTestKind{TestKind: toTestKind(req.kind()), Valid: true},
		ExamYear:         pgtype.Int4{Int32: req.ExamYear, Valid: req.ExamYear > 0},
		DurationMin:      pgtype.Int4{Int32: req.duration(), Valid: true},
		TotalMarks:       nnum(req.TotalMarks),
		PassMarks:        nnum(req.PassingMarks),
		NegativeMarking:  pgtype.Bool{Bool: req.NegativeMarking, Valid: true},
		ShuffleQuestions: pgtype.Bool{Bool: req.ShuffleQuestions, Valid: true},
		IsFree:           pgtype.Bool{Bool: req.IsFree, Valid: true},
		Status:           db.NullPublishStatus{PublishStatus: statusFor(req.IsPublished), Valid: true},
		CreatedBy:        utils.UUIDToPg(creator),
	})
}

// TestView is the frontend-facing shape for a test + its questions.
type TestView struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Kind            string     `json:"kind"`
	Type            string     `json:"type"`
	TestType        string     `json:"test_type"`
	DurationMinutes int32      `json:"duration_minutes"`
	TotalMarks      float64    `json:"total_marks"`
	NegativeMarking bool       `json:"negative_marking"`
	IsPublished     bool       `json:"is_published"`
	Status          string     `json:"status"`
	QuestionCount   int        `json:"question_count"`
	Questions       []Question `json:"questions"`
}

type Question struct {
	ID           string   `json:"id"`
	QuestionText string   `json:"question_text"`
	QuestionType string   `json:"question_type"`
	ImageURL     string   `json:"image_url,omitempty"`
	Marks        float64  `json:"marks"`
	Options      []Option `json:"options"`
}

type Option struct {
	ID         string `json:"id"`
	OptionText string `json:"option_text"`
	ImageURL   string `json:"image_url,omitempty"`
}

func (s *Service) GetTest(ctx context.Context, id uuid.UUID, withQuestions bool) (TestView, error) {
	t, err := s.q.GetTest(ctx, utils.UUIDToPg(id))
	if err != nil {
		return TestView{}, err
	}
	v := TestView{
		ID: utils.UUIDFromPg(t.ID), Title: t.Title, Description: utils.TextFromPg(t.Description),
		Kind: string(t.Kind), Type: fromTestKindLabel(t.Kind), TestType: fromTestKindLabel(t.Kind),
		DurationMinutes: t.DurationMin, TotalMarks: utils.NumericToFloat(t.TotalMarks),
		NegativeMarking: t.NegativeMarking, Status: string(t.Status),
		IsPublished: t.Status == db.PublishStatusPublished,
	}
	tqs, err := s.q.ListTestQuestions(ctx, utils.UUIDToPg(id))
	if err != nil {
		return v, err
	}
	v.QuestionCount = len(tqs)
	if !withQuestions {
		return v, nil
	}
	for _, tq := range tqs {
		q := Question{
			ID: utils.UUIDFromPg(tq.QuestionID), QuestionText: stemText(tq.StemRich),
			QuestionType: fromQuestionKind(tq.Kind), ImageURL: utils.TextFromPg(tq.ImageUrl),
			Marks: utils.NumericToFloat(tq.Marks),
		}
		opts, _ := s.q.ListQuestionOptions(ctx, tq.QuestionID)
		for _, o := range opts {
			q.Options = append(q.Options, Option{
				ID: utils.UUIDFromPg(o.ID), OptionText: stemText(o.BodyRich),
				ImageURL: utils.TextFromPg(o.ImageUrl),
			})
			if q.Options[len(q.Options)-1].OptionText == "" {
				q.Options[len(q.Options)-1].OptionText = utils.TextFromPg(o.Label)
			}
		}
		v.Questions = append(v.Questions, q)
	}
	return v, nil
}

func fromTestKindLabel(k db.TestKind) string {
	switch string(k) {
	case "chapter_test":
		return "chapter"
	case "subject_test":
		return "subject"
	default:
		return string(k)
	}
}

func (s *Service) ListTests(ctx context.Context, tenantID uuid.UUID, courseID *uuid.UUID, limit, offset int32) ([]db.ListTestsRow, error) {
	return s.q.ListTests(ctx, db.ListTestsParams{
		TenantID: utils.UUIDToPg(tenantID), CourseID: utils.UUIDPtrToPg(courseID),
		Limit: limit, Offset: offset,
	})
}

func (s *Service) UpdateTest(ctx context.Context, id uuid.UUID, req CreateTestRequest) error {
	if req.IsPublished {
		return s.SetPublished(ctx, id, true)
	}
	return s.SetPublished(ctx, id, false)
}

func (s *Service) SetPublished(ctx context.Context, id uuid.UUID, published bool) error {
	return s.q.SetTestStatus(ctx, db.SetTestStatusParams{ID: utils.UUIDToPg(id), Status: statusFor(published)})
}

func (s *Service) DeleteTest(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteTest(ctx, utils.UUIDToPg(id))
}

// ── question bank ───────────────────────────────────────────────────

type QuestionOptionRequest struct {
	OptionText   string `json:"option_text"`
	ImageURL     string `json:"image_url"`
	IsCorrect    bool   `json:"is_correct"`
	DisplayOrder int32  `json:"display_order"`
}

type CreateQuestionRequest struct {
	TestID                 *uuid.UUID              `json:"test_id"`
	SubjectID              *uuid.UUID              `json:"subject_id"`
	TopicID                *uuid.UUID              `json:"topic_id"`
	QuestionText           string                  `json:"question_text" validate:"required"`
	QuestionType           string                  `json:"question_type"`
	ImageURL               string                  `json:"image_url"`
	Marks                  float64                 `json:"marks"`
	NegativeMarks          float64                 `json:"negative_marks"`
	Difficulty             string                  `json:"difficulty"`
	Explanation            string                  `json:"explanation"`
	CorrectNumericalAnswer *float64                `json:"correct_numerical_answer"`
	NumericTolerance       *float64                `json:"numeric_tolerance"`
	DisplayOrder           int32                   `json:"display_order"`
	Options                []QuestionOptionRequest `json:"options"`
}

func (s *Service) CreateQuestion(ctx context.Context, tenantID, creator uuid.UUID, req CreateQuestionRequest) (map[string]any, error) {
	diff := req.Difficulty
	if diff == "" {
		diff = "medium"
	}
	marks := req.Marks
	if marks == 0 {
		marks = 1
	}
	q, err := s.q.CreateQuestion(ctx, db.CreateQuestionParams{
		TenantID:         utils.UUIDToPg(tenantID),
		Kind:             toQuestionKind(req.QuestionType),
		SubjectID:        utils.UUIDPtrToPg(req.SubjectID),
		TopicID:          utils.UUIDPtrToPg(req.TopicID),
		StemRich:         stemJSON(req.QuestionText),
		SolutionRich:     stemJSON(req.Explanation),
		ImageUrl:         ntext(req.ImageURL),
		Difficulty:       ntext(diff),
		DefaultMarks:     nnum(marks),
		NegativeMarks:    nnum(req.NegativeMarks),
		NumericAnswer:    numPtr(req.CorrectNumericalAnswer),
		NumericTolerance: numPtr(req.NumericTolerance),
		CreatedBy:        utils.UUIDToPg(creator),
	})
	if err != nil {
		return nil, err
	}
	qid := uuid.UUID(q.ID.Bytes)
	for i, o := range req.Options {
		_, _ = s.q.AddQuestionOption(ctx, db.AddQuestionOptionParams{
			TenantID:     utils.UUIDToPg(tenantID),
			QuestionID:   q.ID,
			Label:        ntext(string(rune('A' + i))),
			BodyRich:     stemJSON(o.OptionText),
			ImageUrl:     ntext(o.ImageURL),
			IsCorrect:    pgtype.Bool{Bool: o.IsCorrect, Valid: true},
			DisplayOrder: pgtype.Int4{Int32: int32(i), Valid: true},
		})
	}
	if req.TestID != nil {
		_ = s.q.AddTestQuestion(ctx, db.AddTestQuestionParams{
			TenantID:     utils.UUIDToPg(tenantID),
			TestID:       utils.UUIDToPg(*req.TestID),
			QuestionID:   q.ID,
			DisplayOrder: pgtype.Int4{Int32: req.DisplayOrder, Valid: true},
			Marks:        nnum(marks),
			Negative:     nnum(req.NegativeMarks),
		})
	}
	return map[string]any{
		"id": qid.String(), "question_text": req.QuestionText,
		"question_type": fromQuestionKind(q.Kind), "difficulty": q.Difficulty,
		"marks": marks,
	}, nil
}

func numPtr(p *float64) pgtype.Numeric {
	if p == nil {
		return pgtype.Numeric{}
	}
	return utils.NumericFromFloat(*p)
}

func (s *Service) DeleteQuestion(ctx context.Context, id uuid.UUID) error {
	_ = s.q.DeleteQuestionOptions(ctx, utils.UUIDToPg(id))
	return s.q.DeleteQuestion(ctx, utils.UUIDToPg(id))
}

// ── attempts ────────────────────────────────────────────────────────

func (s *Service) StartAttempt(ctx context.Context, tenantID, userID, testID uuid.UUID) (map[string]any, error) {
	no, err := s.q.NextAttemptNumber(ctx, db.NextAttemptNumberParams{
		TestID: utils.UUIDToPg(testID), UserID: utils.UUIDToPg(userID),
	})
	if err != nil {
		return nil, err
	}
	cnt, _ := s.q.CountTestQuestions(ctx, utils.UUIDToPg(testID))
	a, err := s.q.CreateTestAttempt(ctx, db.CreateTestAttemptParams{
		TenantID: utils.UUIDToPg(tenantID), TestID: utils.UUIDToPg(testID), UserID: utils.UUIDToPg(userID),
		AttemptNo: int32(no), MaxScore: cnt.TotalMarks,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id": utils.UUIDFromPg(a.ID), "attempt_id": utils.UUIDFromPg(a.ID),
		"attempt_no": a.AttemptNo, "status": string(a.Status),
		"max_score": utils.NumericToFloat(a.MaxScore), "started_at": a.StartedAt.Time,
	}, nil
}

type SubmitAnswerRequest struct {
	AttemptID        uuid.UUID  `json:"attempt_id" validate:"required"`
	QuestionID       uuid.UUID  `json:"question_id" validate:"required"`
	SelectedOptionID *uuid.UUID `json:"selected_option_id"`
	NumericalAnswer  *float64   `json:"numerical_answer"`
	SubjectiveAnswer string     `json:"subjective_answer"`
	TimeTakenSeconds int32      `json:"time_taken_seconds"`
}

func (s *Service) SubmitAnswer(ctx context.Context, tenantID, userID uuid.UUID, req SubmitAnswerRequest) error {
	att, err := s.q.GetTestAttempt(ctx, utils.UUIDToPg(req.AttemptID))
	if err != nil {
		return err
	}
	if uuid.UUID(att.UserID.Bytes) != userID {
		return errors.New("forbidden")
	}
	if string(att.Status) != "in_progress" {
		return errors.New("attempt not in progress")
	}

	q, err := s.q.GetQuestion(ctx, utils.UUIDToPg(req.QuestionID))
	if err != nil {
		return err
	}

	var isCorrect pgtype.Bool
	var marks pgtype.Numeric
	var selected []pgtype.UUID

	switch fromQuestionKind(q.Kind) {
	case "mcq":
		if req.SelectedOptionID != nil {
			selected = []pgtype.UUID{utils.UUIDToPg(*req.SelectedOptionID)}
			correct, _ := s.q.ListCorrectOptionIDs(ctx, utils.UUIDToPg(req.QuestionID))
			ok := false
			for _, cid := range correct {
				if uuid.UUID(cid.Bytes) == *req.SelectedOptionID {
					ok = true
					break
				}
			}
			isCorrect = pgtype.Bool{Bool: ok, Valid: true}
			if ok {
				marks = q.DefaultMarks
			} else {
				marks = negate(q.NegativeMarks)
			}
		}
	case "numerical":
		if req.NumericalAnswer != nil {
			want := utils.NumericToFloat(q.NumericAnswer)
			tol := utils.NumericToFloat(q.NumericTolerance)
			ok := math.Abs(*req.NumericalAnswer-want) <= math.Max(tol, 1e-9)
			isCorrect = pgtype.Bool{Bool: ok, Valid: true}
			if ok {
				marks = q.DefaultMarks
			} else {
				marks = negate(q.NegativeMarks)
			}
		}
	default:
		// subjective — needs manual grading
	}

	return s.q.SaveTestResponse(ctx, db.SaveTestResponseParams{
		TenantID:          utils.UUIDToPg(tenantID),
		AttemptID:         utils.UUIDToPg(req.AttemptID),
		QuestionID:        utils.UUIDToPg(req.QuestionID),
		SelectedOptionIds: selected,
		NumericAnswer:     numPtr(req.NumericalAnswer),
		TextAnswer:        ntext(req.SubjectiveAnswer),
		IsCorrect:         isCorrect,
		Marks:             marks,
		TimeSec:           pgtype.Int4{Int32: req.TimeTakenSeconds, Valid: true},
	})
}

func negate(n pgtype.Numeric) pgtype.Numeric {
	f := utils.NumericToFloat(n)
	if f == 0 {
		return pgtype.Numeric{}
	}
	return utils.NumericFromFloat(-f)
}

func (s *Service) SubmitAttempt(ctx context.Context, userID, attemptID uuid.UUID) (map[string]any, error) {
	att, err := s.q.GetTestAttempt(ctx, utils.UUIDToPg(attemptID))
	if err != nil {
		return nil, err
	}
	if uuid.UUID(att.UserID.Bytes) != userID {
		return nil, errors.New("forbidden")
	}
	sc, err := s.q.ScoreAttemptResponses(ctx, utils.UUIDToPg(attemptID))
	if err != nil {
		return nil, err
	}
	total := s.qCount(ctx, uuid.UUID(att.TestID.Bytes))
	skipped := int32(total) - int32(sc.CorrectCount) - int32(sc.WrongCount)
	if skipped < 0 {
		skipped = 0
	}
	dur := int32(time.Since(att.StartedAt.Time).Seconds())
	fin, err := s.q.FinalizeTestAttempt(ctx, db.FinalizeTestAttemptParams{
		ID:           utils.UUIDToPg(attemptID),
		Score:        sc.Score,
		CorrectCount: int32(sc.CorrectCount),
		WrongCount:   int32(sc.WrongCount),
		SkippedCount: skipped,
		DurationSec:  dur,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id": utils.UUIDFromPg(fin.ID), "score": utils.NumericToFloat(fin.Score),
		"max_score": utils.NumericToFloat(fin.MaxScore), "correct_count": fin.CorrectCount,
		"wrong_count": fin.WrongCount, "skipped_count": fin.SkippedCount,
	}, nil
}

func (s *Service) qCount(ctx context.Context, testID uuid.UUID) int64 {
	c, _ := s.q.CountTestQuestions(ctx, utils.UUIDToPg(testID))
	return c.Count
}

func (s *Service) GetAttempt(ctx context.Context, id uuid.UUID) (map[string]any, error) {
	a, err := s.q.GetTestAttempt(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	resp, _ := s.q.ListLatestResponses(ctx, utils.UUIDToPg(id))
	answers := make([]map[string]any, 0, len(resp))
	for _, r := range resp {
		sel := make([]string, 0, len(r.SelectedOptionIds))
		for _, o := range r.SelectedOptionIds {
			sel = append(sel, utils.UUIDFromPg(o))
		}
		answers = append(answers, map[string]any{
			"question_id": utils.UUIDFromPg(r.QuestionID), "selected_option_ids": sel,
			"numerical_answer":  utils.NumericToFloat(r.NumericAnswer),
			"subjective_answer": utils.TextFromPg(r.TextAnswer),
			"is_correct":        r.IsCorrect.Bool, "marks": utils.NumericToFloat(r.Marks),
		})
	}
	return map[string]any{
		"id": utils.UUIDFromPg(a.ID), "test_id": utils.UUIDFromPg(a.TestID),
		"status": string(a.Status), "score": utils.NumericToFloat(a.Score),
		"max_score": utils.NumericToFloat(a.MaxScore), "correct_count": a.CorrectCount,
		"wrong_count": a.WrongCount, "answers": answers,
	}, nil
}

func (s *Service) ListMyAttempts(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]map[string]any, error) {
	rows, err := s.q.ListAttemptsForUser(ctx, db.ListAttemptsForUserParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"id": utils.UUIDFromPg(r.ID), "test_id": utils.UUIDFromPg(r.TestID),
			"test_title": r.Title, "attempt_no": r.AttemptNo, "status": string(r.Status),
			"score": utils.NumericToFloat(r.Score), "max_score": utils.NumericToFloat(r.MaxScore),
			"correct_count": r.CorrectCount, "wrong_count": r.WrongCount,
			"submitted_at": tval(r.SubmittedAt),
		})
	}
	return out, nil
}

func tval(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}
