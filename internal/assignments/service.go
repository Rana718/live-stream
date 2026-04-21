package assignments

import (
	"context"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type CreateAssignmentRequest struct {
	BatchID       *uuid.UUID `json:"batch_id"`
	CourseID      *uuid.UUID `json:"course_id"`
	ChapterID     *uuid.UUID `json:"chapter_id"`
	TopicID       *uuid.UUID `json:"topic_id"`
	Title         string     `json:"title" validate:"required,min=3"`
	Description   string     `json:"description"`
	AttachmentURL string     `json:"attachment_url"`
	DueDate       *time.Time `json:"due_date"`
	MaxMarks      float64    `json:"max_marks"`
	IsPublished   bool       `json:"is_published"`
}

func (s *Service) Create(ctx context.Context, creator uuid.UUID, req CreateAssignmentRequest) (*db.Assignment, error) {
	if req.MaxMarks == 0 {
		req.MaxMarks = 100
	}
	a, err := s.q.CreateAssignment(ctx, db.CreateAssignmentParams{
		BatchID:       utils.UUIDPtrToPg(req.BatchID),
		CourseID:      utils.UUIDPtrToPg(req.CourseID),
		ChapterID:     utils.UUIDPtrToPg(req.ChapterID),
		TopicID:       utils.UUIDPtrToPg(req.TopicID),
		Title:         req.Title,
		Description:   utils.TextToPg(req.Description),
		AttachmentUrl: utils.TextToPg(req.AttachmentURL),
		DueDate:       utils.TimestampPtrToPg(req.DueDate),
		MaxMarks:      utils.NumericFromFloat(req.MaxMarks),
		IsPublished:   utils.BoolToPg(req.IsPublished),
		CreatedBy:     utils.UUIDToPg(creator),
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*db.Assignment, error) {
	a, err := s.q.GetAssignmentByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Service) ListByBatch(ctx context.Context, batchID uuid.UUID, limit, offset int32) ([]db.Assignment, error) {
	return s.q.ListAssignmentsByBatch(ctx, db.ListAssignmentsByBatchParams{
		BatchID: utils.UUIDToPg(batchID),
		Limit:   limit, Offset: offset,
	})
}

func (s *Service) ListByCourse(ctx context.Context, courseID uuid.UUID, limit, offset int32) ([]db.Assignment, error) {
	return s.q.ListAssignmentsByCourse(ctx, db.ListAssignmentsByCourseParams{
		CourseID: utils.UUIDToPg(courseID),
		Limit:    limit, Offset: offset,
	})
}

func (s *Service) ListMyCreated(ctx context.Context, creatorID uuid.UUID, limit, offset int32) ([]db.Assignment, error) {
	return s.q.ListAssignmentsCreatedBy(ctx, db.ListAssignmentsCreatedByParams{
		CreatedBy: utils.UUIDToPg(creatorID),
		Limit:     limit, Offset: offset,
	})
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req CreateAssignmentRequest) (*db.Assignment, error) {
	a, err := s.q.UpdateAssignment(ctx, db.UpdateAssignmentParams{
		ID:            utils.UUIDToPg(id),
		Title:         req.Title,
		Description:   utils.TextToPg(req.Description),
		AttachmentUrl: utils.TextToPg(req.AttachmentURL),
		DueDate:       utils.TimestampPtrToPg(req.DueDate),
		MaxMarks:      utils.NumericFromFloat(req.MaxMarks),
		IsPublished:   utils.BoolToPg(req.IsPublished),
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteAssignment(ctx, utils.UUIDToPg(id))
}

type SubmitRequest struct {
	AssignmentID   uuid.UUID `json:"assignment_id" validate:"required"`
	SubmissionText string    `json:"submission_text"`
	FilePath       string    `json:"file_path"`
}

func (s *Service) Submit(ctx context.Context, userID uuid.UUID, req SubmitRequest) (*db.AssignmentSubmission, error) {
	sub, err := s.q.SubmitAssignment(ctx, db.SubmitAssignmentParams{
		AssignmentID:   utils.UUIDToPg(req.AssignmentID),
		UserID:         utils.UUIDToPg(userID),
		SubmissionText: utils.TextToPg(req.SubmissionText),
		FilePath:       utils.TextToPg(req.FilePath),
	})
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) GetMySubmission(ctx context.Context, userID, assignmentID uuid.UUID) (*db.AssignmentSubmission, error) {
	sub, err := s.q.GetMySubmission(ctx, db.GetMySubmissionParams{
		AssignmentID: utils.UUIDToPg(assignmentID),
		UserID:       utils.UUIDToPg(userID),
	})
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) ListSubmissions(ctx context.Context, assignmentID uuid.UUID, limit, offset int32) ([]db.ListSubmissionsForAssignmentRow, error) {
	return s.q.ListSubmissionsForAssignment(ctx, db.ListSubmissionsForAssignmentParams{
		AssignmentID: utils.UUIDToPg(assignmentID),
		Limit:        limit, Offset: offset,
	})
}

func (s *Service) ListMySubmissions(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.ListMySubmissionsRow, error) {
	return s.q.ListMySubmissions(ctx, db.ListMySubmissionsParams{
		UserID: utils.UUIDToPg(userID),
		Limit:  limit, Offset: offset,
	})
}

type GradeRequest struct {
	MarksObtained float64 `json:"marks_obtained" validate:"gte=0"`
	Feedback      string  `json:"feedback"`
}

func (s *Service) Grade(ctx context.Context, graderID, submissionID uuid.UUID, req GradeRequest) (*db.AssignmentSubmission, error) {
	sub, err := s.q.GradeSubmission(ctx, db.GradeSubmissionParams{
		ID:            utils.UUIDToPg(submissionID),
		MarksObtained: utils.NumericFromFloat(req.MarksObtained),
		Feedback:      utils.TextToPg(req.Feedback),
		GradedBy:      utils.UUIDToPg(graderID),
	})
	if err != nil {
		return nil, err
	}
	return &sub, nil
}
