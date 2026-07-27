// Integration test for the full test-taking + auto-scoring pipeline:
// create a test with MCQ questions, start an attempt, answer some
// correctly/incorrectly/not at all, submit, and verify the score,
// correct/wrong/skipped counts all come out right. This is the core
// correctness guarantee behind every DPP/mock/PYQ result students see.
//
// Skipped automatically if TEST_DATABASE_URL is unset, same convention as
// internal/courseorders/service_test.go.
package tests_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"live-platform/internal/config"
	"live-platform/internal/database"
	"live-platform/internal/tests"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping")
	}
	pool, err := database.NewPostgresPool(&config.DatabaseConfig{
		Host: "localhost", Port: "5432",
		User: "app_user", Password: "app_user_dev_password",
		DBName: "live_platform", SSLMode: "disable",
	})
	if err != nil {
		t.Fatalf("connect as app_user: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func fixtures(t *testing.T, pool *pgxpool.Pool) (tenantID, userID uuid.UUID) {
	t.Helper()
	superCtx := database.WithSuperAdmin(context.Background())

	tenantID = uuid.New()
	if _, err := pool.Exec(superCtx, `
		INSERT INTO tenants (id, org_code, name, slug, plan, status)
		VALUES ($1, $2, $3, $4, 'starter', 'active')
	`, tenantID, "TS"+tenantID.String()[:6], "ts-"+tenantID.String()[:6], "ts-"+tenantID.String()[:6]); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	userID = uuid.New()
	if _, err := pool.Exec(superCtx, `INSERT INTO users (id, tenant_id, role) VALUES ($1, $2, 'student')`, userID, tenantID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(superCtx, "DELETE FROM tenants WHERE id = $1", tenantID)
	})
	return tenantID, userID
}

func TestFullAttemptFlow_ScoresCorrectly(t *testing.T) {
	pool := testPool(t)
	svc := tests.NewService(pool)
	tenantID, userID := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	test, err := svc.CreateTest(ctx, tenantID, userID, tests.CreateTestRequest{
		Title:           "Scoring Test",
		TestType:        "dpp",
		DurationMinutes: 30,
		NegativeMarking: true,
	})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}
	testID := uuid.UUID(test.ID.Bytes)

	// Q1: will be answered correctly. +4 marks.
	q1, opts1, err := svc.CreateQuestion(ctx, tests.CreateQuestionRequest{
		TestID: testID, QuestionText: "2+2?", QuestionType: "mcq",
		Marks: 4, NegativeMarks: 1, Difficulty: "easy",
		Options: []tests.QuestionOptionRequest{
			{OptionText: "4", IsCorrect: true},
			{OptionText: "5", IsCorrect: false},
		},
	})
	if err != nil {
		t.Fatalf("create Q1: %v", err)
	}
	var q1Correct uuid.UUID
	for _, o := range opts1 {
		if o.IsCorrect.Bool {
			q1Correct = uuid.UUID(o.ID.Bytes)
		}
	}

	// Q2: will be answered incorrectly. -1 mark (negative marking).
	q2, opts2, err := svc.CreateQuestion(ctx, tests.CreateQuestionRequest{
		TestID: testID, QuestionText: "3+3?", QuestionType: "mcq",
		Marks: 4, NegativeMarks: 1, Difficulty: "easy",
		Options: []tests.QuestionOptionRequest{
			{OptionText: "6", IsCorrect: true},
			{OptionText: "7", IsCorrect: false},
		},
	})
	if err != nil {
		t.Fatalf("create Q2: %v", err)
	}
	var q2Wrong uuid.UUID
	for _, o := range opts2 {
		if !o.IsCorrect.Bool {
			q2Wrong = uuid.UUID(o.ID.Bytes)
		}
	}

	// Q3: left unanswered — should count as skipped, zero marks either way.
	_, _, err = svc.CreateQuestion(ctx, tests.CreateQuestionRequest{
		TestID: testID, QuestionText: "4+4?", QuestionType: "mcq",
		Marks: 4, NegativeMarks: 1, Difficulty: "easy",
		Options: []tests.QuestionOptionRequest{
			{OptionText: "8", IsCorrect: true},
			{OptionText: "9", IsCorrect: false},
		},
	})
	if err != nil {
		t.Fatalf("create Q3: %v", err)
	}

	attempt, err := svc.StartAttempt(ctx, userID, testID)
	if err != nil {
		t.Fatalf("StartAttempt: %v", err)
	}
	attemptID := uuid.UUID(attempt.ID.Bytes)
	if attempt.TotalQuestions.Int32 != 3 {
		t.Fatalf("expected total_questions=3, got %d", attempt.TotalQuestions.Int32)
	}

	ans1, err := svc.SubmitAnswer(ctx, userID, tests.SubmitAnswerRequest{
		AttemptID: attemptID, QuestionID: uuid.UUID(q1.ID.Bytes), SelectedOptionID: &q1Correct,
	})
	if err != nil {
		t.Fatalf("SubmitAnswer Q1: %v", err)
	}
	if !ans1.IsCorrect.Bool {
		t.Fatal("expected Q1 to be marked correct")
	}
	if got, _ := ans1.MarksObtained.Float64Value(); got.Float64 != 4 {
		t.Fatalf("expected Q1 marks=4, got %v", got.Float64)
	}

	ans2, err := svc.SubmitAnswer(ctx, userID, tests.SubmitAnswerRequest{
		AttemptID: attemptID, QuestionID: uuid.UUID(q2.ID.Bytes), SelectedOptionID: &q2Wrong,
	})
	if err != nil {
		t.Fatalf("SubmitAnswer Q2: %v", err)
	}
	if ans2.IsCorrect.Bool {
		t.Fatal("expected Q2 to be marked incorrect")
	}
	if got, _ := ans2.MarksObtained.Float64Value(); got.Float64 != -1 {
		t.Fatalf("expected Q2 marks=-1 (negative marking), got %v", got.Float64)
	}

	// Q3 never answered — submit the attempt as-is.
	final, err := svc.SubmitAttempt(ctx, userID, attemptID)
	if err != nil {
		t.Fatalf("SubmitAttempt: %v", err)
	}
	if final.CorrectCount.Int32 != 1 {
		t.Fatalf("expected correct_count=1, got %d", final.CorrectCount.Int32)
	}
	if final.WrongCount.Int32 != 1 {
		t.Fatalf("expected wrong_count=1, got %d", final.WrongCount.Int32)
	}
	if final.SkippedCount.Int32 != 1 {
		t.Fatalf("expected skipped_count=1, got %d", final.SkippedCount.Int32)
	}
	gotScore, _ := final.Score.Float64Value()
	if gotScore.Float64 != 3 { // +4 (Q1) - 1 (Q2) + 0 (Q3 skipped) = 3
		t.Fatalf("expected total score=3, got %v", gotScore.Float64)
	}
}

func TestSubmitAnswer_RejectsWrongUser(t *testing.T) {
	pool := testPool(t)
	svc := tests.NewService(pool)
	tenantID, userID := fixtures(t, pool)
	attackerID := uuid.New()
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	test, err := svc.CreateTest(ctx, tenantID, userID, tests.CreateTestRequest{
		Title: "Auth Test", TestType: "dpp", DurationMinutes: 10,
	})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}
	q, _, err := svc.CreateQuestion(ctx, tests.CreateQuestionRequest{
		TestID: uuid.UUID(test.ID.Bytes), QuestionText: "x?", QuestionType: "mcq",
		Marks: 1, Difficulty: "easy",
		Options: []tests.QuestionOptionRequest{{OptionText: "a", IsCorrect: true}},
	})
	if err != nil {
		t.Fatalf("CreateQuestion: %v", err)
	}
	attempt, err := svc.StartAttempt(ctx, userID, uuid.UUID(test.ID.Bytes))
	if err != nil {
		t.Fatalf("StartAttempt: %v", err)
	}

	_, err = svc.SubmitAnswer(ctx, attackerID, tests.SubmitAnswerRequest{
		AttemptID: uuid.UUID(attempt.ID.Bytes), QuestionID: uuid.UUID(q.ID.Bytes),
	})
	if err == nil {
		t.Fatal("expected forbidden error submitting an answer for someone else's attempt")
	}
}
