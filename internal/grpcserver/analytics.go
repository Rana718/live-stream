package grpcserver

import (
	"context"

	analyticsv1 "live-platform/gen/proto/live/analytics/v1"
	"live-platform/internal/analytics"
	"live-platform/internal/utils"
)

type AnalyticsServer struct {
	analyticsv1.UnimplementedAnalyticsServiceServer
	svc *analytics.Service
}

func NewAnalyticsServer(svc *analytics.Service) *AnalyticsServer { return &AnalyticsServer{svc: svc} }

func clampN(n, def int32) int {
	if n <= 0 || n > 100 {
		return int(def)
	}
	return int(n)
}

func (s *AnalyticsServer) GetMyStats(ctx context.Context, _ *analyticsv1.GetMyStatsRequest) (*analyticsv1.GetMyStatsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	st, err := s.svc.GetUserStats(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &analyticsv1.GetMyStatsResponse{Stats: &analyticsv1.UserStats{
		TotalAttempts: st.TotalAttempts, CompletedAttempts: st.CompletedAttempts, AverageScore: st.AverageScore,
		BestScore: st.BestScore, TotalTimeSeconds: st.TotalTimeSeconds, AvgTimePerQuestionSeconds: st.AvgTimePerQuestion,
		WatchedSeconds: st.WatchedSeconds, CompletedLectures: st.CompletedLectures,
	}}, nil
}

func (s *AnalyticsServer) GetMyWeakTopics(ctx context.Context, req *analyticsv1.GetMyWeakTopicsRequest) (*analyticsv1.GetMyWeakTopicsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.GetWeakTopics(ctx, c.TenantID, c.UserID, clampN(req.GetLimit(), 5))
	if err != nil {
		return nil, toStatus(err)
	}
	out := &analyticsv1.GetMyWeakTopicsResponse{}
	for _, r := range rows {
		out.Topics = append(out.Topics, &analyticsv1.TopicAccuracy{
			TopicId: r.TopicID, TotalAnswers: r.TotalAnswers, CorrectAnswers: r.CorrectAnswers, AccuracyPercent: r.AccuracyPercent,
		})
	}
	return out, nil
}

func (s *AnalyticsServer) GetMyDifficultyBreakdown(ctx context.Context, _ *analyticsv1.GetMyDifficultyBreakdownRequest) (*analyticsv1.GetMyDifficultyBreakdownResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.GetDifficultyBreakdown(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &analyticsv1.GetMyDifficultyBreakdownResponse{}
	for _, r := range rows {
		out.Rows = append(out.Rows, &analyticsv1.DifficultyBreakdown{
			Difficulty: r.Difficulty, TotalAnswers: r.TotalAnswers, CorrectAnswers: r.CorrectAnswers,
		})
	}
	return out, nil
}

func (s *AnalyticsServer) GetMyRecentAttempts(ctx context.Context, req *analyticsv1.GetMyRecentAttemptsRequest) (*analyticsv1.GetMyRecentAttemptsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.GetRecentAttempts(ctx, c.TenantID, c.UserID, int32(clampN(req.GetLimit(), 10)))
	if err != nil {
		return nil, toStatus(err)
	}
	out := &analyticsv1.GetMyRecentAttemptsResponse{}
	for _, r := range rows {
		out.Attempts = append(out.Attempts, &analyticsv1.RecentAttempt{
			Id: r.ID, TestId: r.TestID, TestTitle: r.TestTitle, Score: r.Score, CorrectCount: r.CorrectCount,
			WrongCount: r.WrongCount, TimeTakenSeconds: r.TimeTakenSeconds, Status: r.Status,
		})
	}
	return out, nil
}

func (s *AnalyticsServer) GetTenantDashboard(ctx context.Context, _ *analyticsv1.GetTenantDashboardRequest) (*analyticsv1.GetTenantDashboardResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	d, err := s.svc.TenantDashboard(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &analyticsv1.GetTenantDashboardResponse{Dashboard: &analyticsv1.TenantDashboard{
		TotalCourses: d.TotalCourses, PublishedCourses: d.PublishedCourses, TotalStudents: d.TotalStudents,
		TotalInstructors: d.TotalInstructors, TotalEnrollments: d.TotalEnrollments, RevenueMinor: d.RevenueMinor,
		PaidOrders: d.PaidOrders,
	}}, nil
}

func (s *AnalyticsServer) GetTenantTopCourses(ctx context.Context, req *analyticsv1.GetTenantTopCoursesRequest) (*analyticsv1.GetTenantTopCoursesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	rows, err := s.svc.TenantTopCourses(ctx, c.TenantID, int32(clampN(req.GetLimit(), 10)))
	if err != nil {
		return nil, toStatus(err)
	}
	out := &analyticsv1.GetTenantTopCoursesResponse{}
	for _, r := range rows {
		out.Courses = append(out.Courses, &analyticsv1.TopCourse{
			CourseId: utils.UUIDFromPg(r.CourseID), Title: r.Title, Enrollments: r.Sales, RevenueMinor: r.RevenueMinor,
		})
	}
	return out, nil
}
