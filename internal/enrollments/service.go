package enrollments

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type EnrollRequest struct {
	CourseID uuid.UUID  `json:"course_id" validate:"required"`
	BatchID  *uuid.UUID `json:"batch_id"`
}

func (s *Service) Enroll(ctx context.Context, tenantID, userID uuid.UUID, req EnrollRequest) (*db.Enrollment, error) {
	e, err := s.q.CreateEnrollment(ctx, db.CreateEnrollmentParams{
		TenantID: utils.UUIDToPg(tenantID),
		UserID:   utils.UUIDToPg(userID),
		CourseID: utils.UUIDToPg(req.CourseID),
		BatchID:  utils.UUIDPtrToPg(req.BatchID),
		Status:   utils.TextToPg("active"),
	})
	if err != nil {
		return nil, err
	}
	if req.BatchID != nil {
		_ = s.q.IncrementBatchStudents(ctx, utils.UUIDToPg(*req.BatchID))
	}
	return &e, nil
}

func (s *Service) Get(ctx context.Context, userID, courseID uuid.UUID) (*db.Enrollment, error) {
	e, err := s.q.GetEnrollment(ctx, db.GetEnrollmentParams{
		UserID:   utils.UUIDToPg(userID),
		CourseID: utils.UUIDToPg(courseID),
	})
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Service) ListMine(ctx context.Context, userID uuid.UUID) ([]db.ListUserEnrollmentsRow, error) {
	return s.q.ListUserEnrollments(ctx, utils.UUIDToPg(userID))
}

func (s *Service) ListCourseEnrollments(ctx context.Context, courseID uuid.UUID, limit, offset int32) ([]db.ListCourseEnrollmentsRow, error) {
	return s.q.ListCourseEnrollments(ctx, db.ListCourseEnrollmentsParams{
		CourseID: utils.UUIDToPg(courseID),
		Limit:    limit,
		Offset:   offset,
	})
}

func (s *Service) UpdateProgress(ctx context.Context, id uuid.UUID, progressPercent float64) error {
	return s.q.UpdateEnrollmentProgress(ctx, db.UpdateEnrollmentProgressParams{
		ID:              utils.UUIDToPg(id),
		ProgressPercent: utils.NumericFromFloat(progressPercent),
	})
}

func (s *Service) Cancel(ctx context.Context, userID, courseID uuid.UUID) error {
	return s.q.CancelEnrollment(ctx, db.CancelEnrollmentParams{
		UserID:   utils.UUIDToPg(userID),
		CourseID: utils.UUIDToPg(courseID),
	})
}
