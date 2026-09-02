// Package enrollments — schema-v2. enrollments is a thin per-course progress
// projection tied to an entitlement; progress is progress_bps (0-10000). No
// batch student counter (derive from the roster when needed).
package enrollments

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, q: db.New(pool)} }

type EnrollRequest struct {
	CourseID uuid.UUID  `json:"course_id" validate:"required"`
	BatchID  *uuid.UUID `json:"batch_id"`
}

func (s *Service) Enroll(ctx context.Context, tenantID, userID uuid.UUID, req EnrollRequest) (db.UpsertEnrollmentRow, error) {
	return s.q.UpsertEnrollment(ctx, db.UpsertEnrollmentParams{
		TenantID: utils.UUIDToPg(tenantID),
		UserID:   utils.UUIDToPg(userID),
		CourseID: utils.UUIDToPg(req.CourseID),
		BatchID:  utils.UUIDPtrToPg(req.BatchID),
	})
}

func (s *Service) Get(ctx context.Context, tenantID, userID, courseID uuid.UUID) (db.GetEnrollmentRow, error) {
	return s.q.GetEnrollment(ctx, db.GetEnrollmentParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), CourseID: utils.UUIDToPg(courseID),
	})
}

func (s *Service) ListMine(ctx context.Context, tenantID, userID uuid.UUID) ([]db.ListEnrollmentsForUserRow, error) {
	return s.q.ListEnrollmentsForUser(ctx, db.ListEnrollmentsForUserParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
}

func (s *Service) ListRoster(ctx context.Context, tenantID, courseID uuid.UUID, limit, offset int32) ([]db.ListCourseRosterRow, error) {
	return s.q.ListCourseRoster(ctx, db.ListCourseRosterParams{
		TenantID: utils.UUIDToPg(tenantID), CourseID: utils.UUIDToPg(courseID), Limit: limit, Offset: offset,
	})
}

func (s *Service) UpdateProgress(ctx context.Context, tenantID, userID, courseID uuid.UUID, progressPercent float64) error {
	bps := int32(progressPercent * 100)
	if bps < 0 {
		bps = 0
	}
	if bps > 10000 {
		bps = 10000
	}
	return s.q.SetEnrollmentProgress(ctx, db.SetEnrollmentProgressParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
		CourseID: utils.UUIDToPg(courseID), ProgressBps: bps,
	})
}

func (s *Service) Cancel(ctx context.Context, tenantID, userID, courseID uuid.UUID) error {
	return s.q.CancelEnrollment(ctx, db.CancelEnrollmentParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID), CourseID: utils.UUIDToPg(courseID),
	})
}
