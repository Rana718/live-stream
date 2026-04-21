package batches

import (
	"context"
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

type CreateBatchRequest struct {
	CourseID     uuid.UUID  `json:"course_id" validate:"required"`
	Name         string     `json:"name" validate:"required"`
	Description  string     `json:"description"`
	InstructorID *uuid.UUID `json:"instructor_id"`
	StartDate    *time.Time `json:"start_date"`
	EndDate      *time.Time `json:"end_date"`
	MaxStudents  int32      `json:"max_students"`
	IsActive     bool       `json:"is_active"`
}

func (s *Service) Create(ctx context.Context, req CreateBatchRequest) (*db.Batch, error) {
	b, err := s.q.CreateBatch(ctx, db.CreateBatchParams{
		CourseID:     utils.UUIDToPg(req.CourseID),
		Name:         req.Name,
		Description:  utils.TextToPg(req.Description),
		InstructorID: utils.UUIDPtrToPg(req.InstructorID),
		StartDate:    utils.DateToPg(deref(req.StartDate)),
		EndDate:      utils.DateToPg(deref(req.EndDate)),
		MaxStudents:  utils.Int4ToPg(req.MaxStudents),
		IsActive:     utils.BoolToPg(req.IsActive),
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func deref(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*db.Batch, error) {
	b, err := s.q.GetBatchByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Service) ListByCourse(ctx context.Context, courseID uuid.UUID) ([]db.Batch, error) {
	return s.q.ListBatchesByCourse(ctx, utils.UUIDToPg(courseID))
}

func (s *Service) ListByInstructor(ctx context.Context, instructorID uuid.UUID) ([]db.Batch, error) {
	return s.q.ListBatchesByInstructor(ctx, utils.UUIDToPg(instructorID))
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req CreateBatchRequest) (*db.Batch, error) {
	b, err := s.q.UpdateBatch(ctx, db.UpdateBatchParams{
		ID:           utils.UUIDToPg(id),
		Name:         req.Name,
		Description:  utils.TextToPg(req.Description),
		InstructorID: utils.UUIDPtrToPg(req.InstructorID),
		StartDate:    utils.DateToPg(deref(req.StartDate)),
		EndDate:      utils.DateToPg(deref(req.EndDate)),
		MaxStudents:  utils.Int4ToPg(req.MaxStudents),
		IsActive:     utils.BoolToPg(req.IsActive),
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteBatch(ctx, utils.UUIDToPg(id))
}
