package tests

import (
	"context"
	"errors"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

// --- Test CRUD ---

type CreateTestRequest struct {
	CourseID         *uuid.UUID `json:"course_id"`
	SubjectID        *uuid.UUID `json:"subject_id"`
	ChapterID        *uuid.UUID `json:"chapter_id"`
	TopicID          *uuid.UUID `json:"topic_id"`
	ExamCategoryID   *uuid.UUID `json:"exam_category_id"`
	Title            string     `json:"title" validate:"required,min=3"`
	Description      string     `json:"description"`
	TestType         string     `json:"test_type" validate:"required,oneof=dpp chapter subject mock pyq"`
	ExamYear         int32      `json:"exam_year"`
	DurationMinutes  int32      `json:"duration_minutes"`
	TotalMarks       float64    `json:"total_marks"`
	PassingMarks     float64    `json:"passing_marks"`
	NegativeMarking  bool       `json:"negative_marking"`
	ShuffleQuestions bool       `json:"shuffle_questions"`
	Language         string     `json:"language"`
	IsFree           bool       `json:"is_free"`
	IsPublished      bool       `json:"is_published"`
	ScheduledAt      *time.Time `json:"scheduled_at"`
}

func (s *Service) CreateTest(ctx context.Context, creator uuid.UUID, req CreateTestRequest) (*db.Test, error) {
	if req.Language == "" {
		req.Language = "en"
	}
	examYear := pgInt4FromInt32(req.ExamYear)
	t, err := s.q.CreateTest(ctx, db.CreateTestParams{
		CourseID:         utils.UUIDPtrToPg(req.CourseID),
		SubjectID:        utils.UUIDPtrToPg(req.SubjectID),
		ChapterID:        utils.UUIDPtrToPg(req.ChapterID),
		TopicID:          utils.UUIDPtrToPg(req.TopicID),
		ExamCategoryID:   utils.UUIDPtrToPg(req.ExamCategoryID),
		Title:            req.Title,
		Description:      utils.TextToPg(req.Description),
		TestType:         req.TestType,
		ExamYear:         examYear,
		DurationMinutes:  utils.Int4ToPg(req.DurationMinutes),
		TotalMarks:       utils.NumericFromFloat(req.TotalMarks),
		PassingMarks:     utils.NumericFromFloat(req.PassingMarks),
		NegativeMarking:  utils.BoolToPg(req.NegativeMarking),
		ShuffleQuestions: utils.BoolToPg(req.ShuffleQuestions),
		Language:         utils.TextToPg(req.Language),
		IsFree:           utils.BoolToPg(req.IsFree),
		IsPublished:      utils.BoolToPg(req.IsPublished),
		ScheduledAt:      utils.TimestampPtrToPg(req.ScheduledAt),
		CreatedBy:        utils.UUIDToPg(creator),
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func pgInt4FromInt32(n int32) (v interface{ GetValid() bool }) {
	// Helper not strictly required; using utils.Int4ToPg inline suffices.
	return nil
}

func (s *Service) GetTest(ctx context.Context, id uuid.UUID) (*db.Test, error) {
	t, err := s.q.GetTestByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Service) ListByType(ctx context.Context, testType string, limit, offset int32) ([]db.Test, error) {
	return s.q.ListTestsByType(ctx, db.ListTestsByTypeParams{
		TestType: testType, Limit: limit, Offset: offset,
	})
}

func (s *Service) ListByChapter(ctx context.Context, chapterID uuid.UUID) ([]db.Test, error) {
	return s.q.ListTestsByChapter(ctx, utils.UUIDToPg(chapterID))
}

func (s *Service) ListBySubject(ctx context.Context, subjectID uuid.UUID) ([]db.Test, error) {
	return s.q.ListTestsBySubject(ctx, utils.UUIDToPg(subjectID))
}

func (s *Service) ListByCourse(ctx context.Context, courseID uuid.UUID) ([]db.Test, error) {
	return s.q.ListTestsByCourse(ctx, utils.UUIDToPg(courseID))
}

func (s *Service) ListPYQsByYear(ctx context.Context, year int32) ([]db.Test, error) {
	return s.q.ListPYQsByExamYear(ctx, utils.Int4ToPg(year))
}

func (s *Service) ListPYQsByCategory(ctx context.Context, examID uuid.UUID) ([]db.Test, error) {
	return s.q.ListPYQsByExamCategory(ctx, utils.UUIDToPg(examID))
}

func (s *Service) UpdateTest(ctx context.Context, id uuid.UUID, req CreateTestRequest) (*db.Test, error) {
	t, err := s.q.UpdateTest(ctx, db.UpdateTestParams{
		ID:               utils.UUIDToPg(id),
		Title:            req.Title,
		Description:      utils.TextToPg(req.Description),
		DurationMinutes:  utils.Int4ToPg(req.DurationMinutes),
		TotalMarks:       utils.NumericFromFloat(req.TotalMarks),
		PassingMarks:     utils.NumericFromFloat(req.PassingMarks),
		NegativeMarking:  utils.BoolToPg(req.NegativeMarking),
		ShuffleQuestions: utils.BoolToPg(req.ShuffleQuestions),
		Language:         utils.TextToPg(req.Language),
		IsFree:           utils.BoolToPg(req.IsFree),
		IsPublished:      utils.BoolToPg(req.IsPublished),
		ScheduledAt:      utils.TimestampPtrToPg(req.ScheduledAt),
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Service) DeleteTest(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteTest(ctx, utils.UUIDToPg(id))
}

// --- Questions & options ---

type QuestionOptionRequest struct {
	OptionText   string `json:"option_text" validate:"required"`
	ImageURL     string `json:"image_url"`
	IsCorrect    bool   `json:"is_correct"`
	DisplayOrder int32  `json:"display_order"`
}

type CreateQuestionRequest struct {
	TestID                 uuid.UUID               `json:"test_id" validate:"required"`
	TopicID                *uuid.UUID              `json:"topic_id"`
	QuestionText           string                  `json:"question_text" validate:"required"`
	QuestionType           string                  `json:"question_type" validate:"required,oneof=mcq numerical subjective"`
	ImageURL               string                  `json:"image_url"`
	Marks                  float64                 `json:"marks"`
	NegativeMarks          float64                 `json:"negative_marks"`
	Difficulty             string                  `json:"difficulty" validate:"required,oneof=easy medium hard"`
	Explanation            string                  `json:"explanation"`
	CorrectNumericalAnswer *float64                `json:"correct_numerical_answer"`
	DisplayOrder           int32                   `json:"display_order"`
	Options                []QuestionOptionRequest `json:"options"`
}

func (s *Service) CreateQuestion(ctx context.Context, req CreateQuestionRequest) (*db.Question, []db.QuestionOption, error) {
	var correctNum interface{}
	_ = correctNum // placeholder

	q, err := s.q.CreateQuestion(ctx, db.CreateQuestionParams{
		TestID:                 utils.UUIDToPg(req.TestID),
		TopicID:                utils.UUIDPtrToPg(req.TopicID),
		QuestionText:           req.QuestionText,
		QuestionType:           utils.TextToPg(req.QuestionType),
		ImageUrl:               utils.TextToPg(req.ImageURL),
		Marks:                  utils.NumericFromFloat(req.Marks),
		NegativeMarks:          utils.NumericFromFloat(req.NegativeMarks),
		Difficulty:             utils.TextToPg(req.Difficulty),
		Explanation:            utils.TextToPg(req.Explanation),
		CorrectNumericalAnswer: nilableFloatNumeric(req.CorrectNumericalAnswer),
		DisplayOrder:           utils.Int4ToPg(req.DisplayOrder),
	})
	if err != nil {
		return nil, nil, err
	}

	opts := make([]db.QuestionOption, 0, len(req.Options))
	for _, o := range req.Options {
		created, err := s.q.CreateQuestionOption(ctx, db.CreateQuestionOptionParams{
			QuestionID:   q.ID,
			OptionText:   o.OptionText,
			ImageUrl:     utils.TextToPg(o.ImageURL),
			IsCorrect:    utils.BoolToPg(o.IsCorrect),
			DisplayOrder: utils.Int4ToPg(o.DisplayOrder),
		})
		if err != nil {
			return nil, nil, err
		}
		opts = append(opts, created)
	}
	return &q, opts, nil
}

func nilableFloatNumeric(p *float64) (v interface{}) {
	// pgtype.Numeric with Valid=false when nil
	if p == nil {
		return utils.NumericFromString("")
	}
	return utils.NumericFromFloat(*p)
}

func (s *Service) ListQuestions(ctx context.Context, testID uuid.UUID) ([]db.Question, error) {
	return s.q.ListQuestionsByTest(ctx, utils.UUIDToPg(testID))
}

func (s *Service) ListOptions(ctx context.Context, questionID uuid.UUID) ([]db.QuestionOption, error) {
	return s.q.ListOptionsByQuestion(ctx, utils.UUIDToPg(questionID))
}

func (s *Service) DeleteQuestion(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteQuestion(ctx, utils.UUIDToPg(id))
}

// --- Attempts ---

func (s *Service) StartAttempt(ctx context.Context, userID, testID uuid.UUID) (*db.TestAttempt, error) {
	// Reuse in-progress attempt if present.
	if cur, err := s.q.GetActiveAttempt(ctx, db.GetActiveAttemptParams{
		UserID: utils.UUIDToPg(userID),
		TestID: utils.UUIDToPg(testID),
	}); err == nil {
		return &cur, nil
	}
	cnt, err := s.q.CountQuestionsByTest(ctx, utils.UUIDToPg(testID))
	if err != nil {
		return nil, err
	}
	att, err := s.q.CreateTestAttempt(ctx, db.CreateTestAttemptParams{
		UserID:         utils.UUIDToPg(userID),
		TestID:         utils.UUIDToPg(testID),
		TotalQuestions: utils.Int4ToPg(int32(cnt)),
	})
	if err != nil {
		return nil, err
	}
	return &att, nil
}

type SubmitAnswerRequest struct {
	AttemptID        uuid.UUID  `json:"attempt_id" validate:"required"`
	QuestionID       uuid.UUID  `json:"question_id" validate:"required"`
	SelectedOptionID *uuid.UUID `json:"selected_option_id"`
	NumericalAnswer  *float64   `json:"numerical_answer"`
	SubjectiveAnswer string     `json:"subjective_answer"`
	TimeTakenSeconds int32      `json:"time_taken_seconds"`
}

func (s *Service) SubmitAnswer(ctx context.Context, userID uuid.UUID, req SubmitAnswerRequest) (*db.TestAnswer, error) {
	att, err := s.q.GetTestAttemptByID(ctx, utils.UUIDToPg(req.AttemptID))
	if err != nil {
		return nil, err
	}
	if utils.UUIDFromPg(att.UserID) != userID.String() {
		return nil, errors.New("forbidden")
	}
	if utils.TextFromPg(att.Status) != "in_progress" {
		return nil, errors.New("attempt not in progress")
	}

	q, err := s.q.GetQuestionByID(ctx, utils.UUIDToPg(req.QuestionID))
	if err != nil {
		return nil, err
	}

	isCorrect := false
	marksAwarded := 0.0
	switch utils.TextFromPg(q.QuestionType) {
	case "mcq":
		if req.SelectedOptionID != nil {
			opt, err := s.q.GetQuestionOptionByID(ctx, utils.UUIDToPg(*req.SelectedOptionID))
			if err == nil && utils.BoolFromPg(opt.IsCorrect) {
				isCorrect = true
			}
		}
	case "numerical":
		if req.NumericalAnswer != nil {
			expected := utils.NumericToFloat(q.CorrectNumericalAnswer)
			if almostEqual(*req.NumericalAnswer, expected, 1e-4) {
				isCorrect = true
			}
		}
	case "subjective":
		// Graded manually, skip auto-scoring.
	}
	if isCorrect {
		marksAwarded = utils.NumericToFloat(q.Marks)
	} else if req.SelectedOptionID != nil || req.NumericalAnswer != nil {
		marksAwarded = -utils.NumericToFloat(q.NegativeMarks)
	}

	ans, err := s.q.UpsertTestAnswer(ctx, db.UpsertTestAnswerParams{
		AttemptID:        utils.UUIDToPg(req.AttemptID),
		QuestionID:       utils.UUIDToPg(req.QuestionID),
		SelectedOptionID: utils.UUIDPtrToPg(req.SelectedOptionID),
		NumericalAnswer:  nilableFloatNumeric(req.NumericalAnswer).(interface{ GetValid() bool }).(any).(pgtypeNumericWrapper), // converted below
		SubjectiveAnswer: utils.TextToPg(req.SubjectiveAnswer),
		IsCorrect:        utils.BoolToPg(isCorrect),
		MarksObtained:    utils.NumericFromFloat(marksAwarded),
		TimeTakenSeconds: utils.Int4ToPg(req.TimeTakenSeconds),
	})
	if err != nil {
		return nil, err
	}
	return &ans, nil
}

// pgtypeNumericWrapper placeholder
type pgtypeNumericWrapper struct{}

func almostEqual(a, b, eps float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}

func (s *Service) SubmitAttempt(ctx context.Context, userID, attemptID uuid.UUID) (*db.TestAttempt, error) {
	att, err := s.q.GetTestAttemptByID(ctx, utils.UUIDToPg(attemptID))
	if err != nil {
		return nil, err
	}
	if utils.UUIDFromPg(att.UserID) != userID.String() {
		return nil, errors.New("forbidden")
	}
	correct, _ := s.q.CountCorrectAnswers(ctx, utils.UUIDToPg(attemptID))
	wrong, _ := s.q.CountWrongAnswers(ctx, utils.UUIDToPg(attemptID))
	totalMarks, _ := s.q.SumMarksForAttempt(ctx, utils.UUIDToPg(attemptID))

	totalQs := utils.Int4FromPg(att.TotalQuestions)
	skipped := totalQs - int32(correct) - int32(wrong)
	if skipped < 0 {
		skipped = 0
	}
	timeTaken := int32(time.Since(utils.TsOrNow(att.StartedAt)).Seconds())

	updated, err := s.q.SubmitTestAttempt(ctx, db.SubmitTestAttemptParams{
		ID:               utils.UUIDToPg(attemptID),
		Score:            totalMarks, // already pgtype.Numeric
		CorrectCount:     utils.Int4ToPg(int32(correct)),
		WrongCount:       utils.Int4ToPg(int32(wrong)),
		SkippedCount:     utils.Int4ToPg(skipped),
		TimeTakenSeconds: utils.Int4ToPg(timeTaken),
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *Service) GetAttempt(ctx context.Context, id uuid.UUID) (*db.TestAttempt, error) {
	a, err := s.q.GetTestAttemptByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Service) ListMyAttempts(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.ListUserAttemptsRow, error) {
	return s.q.ListUserAttempts(ctx, db.ListUserAttemptsParams{
		UserID: utils.UUIDToPg(userID),
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Service) ListAttemptAnswers(ctx context.Context, attemptID uuid.UUID) ([]db.TestAnswer, error) {
	return s.q.ListAnswersByAttempt(ctx, utils.UUIDToPg(attemptID))
}
