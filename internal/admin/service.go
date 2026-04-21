package admin

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type DashboardStats struct {
	TotalStudents         int64   `json:"total_students"`
	TotalInstructors      int64   `json:"total_instructors"`
	TotalUsers            int64   `json:"total_users"`
	TotalCourses          int64   `json:"total_courses"`
	PendingApproval       int64   `json:"pending_approval"`
	ActiveBatches         int64   `json:"active_batches"`
	ActiveEnrollments     int64   `json:"active_enrollments"`
	LiveStreams           int64   `json:"live_streams"`
	TotalTests            int64   `json:"total_tests"`
	TotalAttempts         int64   `json:"total_attempts"`
	TotalRevenueCaptured  float64 `json:"total_revenue_captured"`
}

func (s *Service) DashboardStats(ctx context.Context) (*DashboardStats, error) {
	r, err := s.q.AdminDashboardStats(ctx)
	if err != nil {
		return nil, err
	}
	return &DashboardStats{
		TotalStudents:        r.TotalStudents,
		TotalInstructors:     r.TotalInstructors,
		TotalUsers:           r.TotalUsers,
		TotalCourses:         r.TotalCourses,
		PendingApproval:      r.PendingApproval,
		ActiveBatches:        r.ActiveBatches,
		ActiveEnrollments:    r.ActiveEnrollments,
		LiveStreams:          r.LiveStreams,
		TotalTests:           r.TotalTests,
		TotalAttempts:        r.TotalAttempts,
		TotalRevenueCaptured: utils.NumericToFloat(r.TotalRevenueCaptured),
	}, nil
}

type BatchAttendanceAgg struct {
	BatchID           string  `json:"batch_id"`
	Total             int64   `json:"total"`
	Attended          int64   `json:"attended"`
	AttendancePercent float64 `json:"attendance_percent"`
}

func (s *Service) BatchAttendance(ctx context.Context) ([]BatchAttendanceAgg, error) {
	rows, err := s.q.AttendanceAggregateByBatch(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BatchAttendanceAgg, 0, len(rows))
	for _, r := range rows {
		out = append(out, BatchAttendanceAgg{
			BatchID:           utils.UUIDFromPg(r.BatchID),
			Total:             r.Total,
			Attended:          r.Attended,
			AttendancePercent: utils.NumericToFloat(r.AttendancePercent),
		})
	}
	return out, nil
}

func (s *Service) ListAllUsers(ctx context.Context, role string, limit, offset int32) ([]db.AdminListAllUsersRow, error) {
	return s.q.AdminListAllUsers(ctx, db.AdminListAllUsersParams{
		Column1: role, Limit: limit, Offset: offset,
	})
}

// --- Course approval ---

func (s *Service) ApproveCourse(ctx context.Context, courseID, adminID uuid.UUID) (*db.Course, error) {
	c, err := s.q.ApproveCourse(ctx, db.ApproveCourseParams{
		ID:         utils.UUIDToPg(courseID),
		ApprovedBy: utils.UUIDToPg(adminID),
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) RejectCourse(ctx context.Context, courseID, adminID uuid.UUID, reason string) (*db.Course, error) {
	c, err := s.q.RejectCourse(ctx, db.RejectCourseParams{
		ID:              utils.UUIDToPg(courseID),
		ApprovedBy:      utils.UUIDToPg(adminID),
		RejectionReason: utils.TextToPg(reason),
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) ListPendingApproval(ctx context.Context, limit, offset int32) ([]db.Course, error) {
	return s.q.ListPendingApprovalCourses(ctx, db.ListPendingApprovalCoursesParams{
		Limit: limit, Offset: offset,
	})
}
