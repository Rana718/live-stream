package analytics

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type UserStats struct {
	TotalAttempts       int64   `json:"total_attempts"`
	CompletedAttempts   int64   `json:"completed_attempts"`
	AverageScore        float64 `json:"average_score"`
	BestScore           float64 `json:"best_score"`
	TotalTimeSeconds    int64   `json:"total_time_seconds"`
	AvgTimePerQuestion  float64 `json:"avg_time_per_question_seconds"`
	WatchedSeconds      int64   `json:"watched_seconds"`
	CompletedLectures   int64   `json:"completed_lectures"`
}

func (s *Service) GetUserStats(ctx context.Context, userID uuid.UUID) (*UserStats, error) {
	u := utils.UUIDToPg(userID)

	ast, err := s.q.UserAttemptStats(ctx, u)
	if err != nil {
		return nil, err
	}
	avgSec, _ := s.q.UserAvgTimePerQuestion(ctx, u)
	watched, _ := s.q.UserWatchedSeconds(ctx, u)
	lectures, _ := s.q.UserCompletedLectureCount(ctx, u)

	return &UserStats{
		TotalAttempts:      ast.TotalAttempts,
		CompletedAttempts:  ast.CompletedAttempts,
		AverageScore:       utils.NumericToFloat(ast.AvgScore),
		BestScore:          utils.NumericToFloat(ast.BestScore),
		TotalTimeSeconds:   ast.TotalTimeSeconds,
		AvgTimePerQuestion: utils.NumericToFloat(avgSec),
		WatchedSeconds:     watched,
		CompletedLectures:  lectures,
	}, nil
}

type TopicAccuracy struct {
	TopicID         string  `json:"topic_id"`
	TotalAnswers    int64   `json:"total_answers"`
	CorrectAnswers  int64   `json:"correct_answers"`
	AccuracyPercent float64 `json:"accuracy_percent"`
}

func (s *Service) GetWeakTopics(ctx context.Context, userID uuid.UUID, limit int) ([]TopicAccuracy, error) {
	rows, err := s.q.UserTopicAccuracy(ctx, utils.UUIDToPg(userID))
	if err != nil {
		return nil, err
	}
	out := make([]TopicAccuracy, 0, len(rows))
	for _, r := range rows {
		out = append(out, TopicAccuracy{
			TopicID:         utils.UUIDFromPg(r.TopicID),
			TotalAnswers:    r.TotalAnswers,
			CorrectAnswers:  r.CorrectAnswers,
			AccuracyPercent: float64(r.AccuracyPercent),
		})
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type DifficultyBreakdown struct {
	Difficulty     string `json:"difficulty"`
	TotalAnswers   int64  `json:"total_answers"`
	CorrectAnswers int64  `json:"correct_answers"`
}

func (s *Service) GetDifficultyBreakdown(ctx context.Context, userID uuid.UUID) ([]DifficultyBreakdown, error) {
	rows, err := s.q.UserDifficultyAccuracy(ctx, utils.UUIDToPg(userID))
	if err != nil {
		return nil, err
	}
	out := make([]DifficultyBreakdown, 0, len(rows))
	for _, r := range rows {
		out = append(out, DifficultyBreakdown{
			Difficulty:     utils.TextFromPg(r.Difficulty),
			TotalAnswers:   r.TotalAnswers,
			CorrectAnswers: r.CorrectAnswers,
		})
	}
	return out, nil
}

type RecentAttempt struct {
	ID                string  `json:"id"`
	TestID            string  `json:"test_id"`
	TestTitle         string  `json:"test_title"`
	Score             float64 `json:"score"`
	CorrectCount      int32   `json:"correct_count"`
	WrongCount        int32   `json:"wrong_count"`
	TimeTakenSeconds  int32   `json:"time_taken_seconds"`
}

// TenantDashboard returns the headline numbers shown on the tenant_admin
// dashboard. Tenant scoping comes from RLS — the call must run inside
// TenantContext middleware which has set app.tenant_id.
func (s *Service) TenantDashboard(ctx context.Context) (*db.TenantDashboardStatsRow, error) {
	row, err := s.q.TenantDashboardStats(ctx)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) TenantRevenueDaily(ctx context.Context) ([]db.TenantRevenueDailyRow, error) {
	return s.q.TenantRevenueDaily(ctx)
}

func (s *Service) TenantTopCourses(ctx context.Context, limit int32) ([]db.TenantTopCoursesRow, error) {
	return s.q.TenantTopCourses(ctx, limit)
}

func (s *Service) GetRecentAttempts(ctx context.Context, userID uuid.UUID, limit int32) ([]RecentAttempt, error) {
	rows, err := s.q.UserRecentAttempts(ctx, db.UserRecentAttemptsParams{
		UserID: utils.UUIDToPg(userID),
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RecentAttempt, 0, len(rows))
	for _, r := range rows {
		out = append(out, RecentAttempt{
			ID:               utils.UUIDFromPg(r.ID),
			TestID:           utils.UUIDFromPg(r.TestID),
			TestTitle:        r.TestTitle,
			Score:            utils.NumericToFloat(r.Score),
			CorrectCount:     utils.Int4FromPg(r.CorrectCount),
			WrongCount:       utils.Int4FromPg(r.WrongCount),
			TimeTakenSeconds: utils.Int4FromPg(r.TimeTakenSeconds),
		})
	}
	return out, nil
}
