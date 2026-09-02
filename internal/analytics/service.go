// Package analytics — schema-v2. Learner progress from test_attempts /
// test_responses / content_progress; tenant dashboards from orders/order_items.
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
	TotalAttempts      int64   `json:"total_attempts"`
	CompletedAttempts  int64   `json:"completed_attempts"`
	AverageScore       float64 `json:"average_score"`
	BestScore          float64 `json:"best_score"`
	TotalTimeSeconds   int64   `json:"total_time_seconds"`
	AvgTimePerQuestion float64 `json:"avg_time_per_question_seconds"`
	WatchedSeconds     int64   `json:"watched_seconds"`
	CompletedLectures  int64   `json:"completed_lectures"`
}

func (s *Service) GetUserStats(ctx context.Context, tenantID, userID uuid.UUID) (*UserStats, error) {
	t, u := utils.UUIDToPg(tenantID), utils.UUIDToPg(userID)
	ast, err := s.q.UserAttemptStats(ctx, db.UserAttemptStatsParams{TenantID: t, UserID: u})
	if err != nil {
		return nil, err
	}
	avgSec, _ := s.q.UserAvgTimePerQuestion(ctx, db.UserAvgTimePerQuestionParams{TenantID: t, UserID: u})
	watched, _ := s.q.UserWatchedSeconds(ctx, db.UserWatchedSecondsParams{TenantID: t, UserID: u})
	lessons, _ := s.q.UserCompletedLessonCount(ctx, db.UserCompletedLessonCountParams{TenantID: t, UserID: u})
	return &UserStats{
		TotalAttempts:      ast.TotalAttempts,
		CompletedAttempts:  ast.CompletedAttempts,
		AverageScore:       utils.NumericToFloat(ast.AvgScore),
		BestScore:          utils.NumericToFloat(ast.BestScore),
		TotalTimeSeconds:   ast.TotalTimeSeconds,
		AvgTimePerQuestion: utils.NumericToFloat(avgSec),
		WatchedSeconds:     watched,
		CompletedLectures:  lessons,
	}, nil
}

type TopicAccuracy struct {
	TopicID         string  `json:"topic_id"`
	TotalAnswers    int64   `json:"total_answers"`
	CorrectAnswers  int64   `json:"correct_answers"`
	AccuracyPercent float64 `json:"accuracy_percent"`
}

func (s *Service) GetWeakTopics(ctx context.Context, tenantID, userID uuid.UUID, limit int) ([]TopicAccuracy, error) {
	rows, err := s.q.UserTopicAccuracy(ctx, db.UserTopicAccuracyParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]TopicAccuracy, 0, len(rows))
	for _, r := range rows {
		out = append(out, TopicAccuracy{
			TopicID: utils.UUIDFromPg(r.TopicID), TotalAnswers: r.TotalAnswers,
			CorrectAnswers: r.CorrectAnswers, AccuracyPercent: r.AccuracyPercent,
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

func (s *Service) GetDifficultyBreakdown(ctx context.Context, tenantID, userID uuid.UUID) ([]DifficultyBreakdown, error) {
	rows, err := s.q.UserDifficultyAccuracy(ctx, db.UserDifficultyAccuracyParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]DifficultyBreakdown, 0, len(rows))
	for _, r := range rows {
		out = append(out, DifficultyBreakdown{
			Difficulty: r.Difficulty, TotalAnswers: r.TotalAnswers, CorrectAnswers: r.CorrectAnswers,
		})
	}
	return out, nil
}

type RecentAttempt struct {
	ID               string  `json:"id"`
	TestID           string  `json:"test_id"`
	TestTitle        string  `json:"test_title"`
	Score            float64 `json:"score"`
	CorrectCount     int32   `json:"correct_count"`
	WrongCount       int32   `json:"wrong_count"`
	TimeTakenSeconds int32   `json:"time_taken_seconds"`
	Status           string  `json:"status"`
}

func (s *Service) GetRecentAttempts(ctx context.Context, tenantID, userID uuid.UUID, limit int32) ([]RecentAttempt, error) {
	rows, err := s.q.UserRecentAttempts(ctx, db.UserRecentAttemptsParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RecentAttempt, 0, len(rows))
	for _, r := range rows {
		out = append(out, RecentAttempt{
			ID: utils.UUIDFromPg(r.ID), TestID: utils.UUIDFromPg(r.TestID), TestTitle: r.TestTitle,
			Score: utils.NumericToFloat(r.Score), CorrectCount: r.CorrectCount,
			WrongCount: r.WrongCount, TimeTakenSeconds: r.DurationSec, Status: string(r.Status),
		})
	}
	return out, nil
}

func (s *Service) TenantDashboard(ctx context.Context, tenantID uuid.UUID) (db.TenantDashboardStatsRow, error) {
	return s.q.TenantDashboardStats(ctx, utils.UUIDToPg(tenantID))
}

func (s *Service) TenantRevenueDaily(ctx context.Context, tenantID uuid.UUID) ([]db.TenantRevenueDailyRow, error) {
	return s.q.TenantRevenueDaily(ctx, utils.UUIDToPg(tenantID))
}

func (s *Service) TenantTopCourses(ctx context.Context, tenantID uuid.UUID, limit int32) ([]db.TenantTopCoursesRow, error) {
	return s.q.TenantTopCourses(ctx, db.TenantTopCoursesParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit,
	})
}
