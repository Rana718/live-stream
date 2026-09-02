// Package assignments — schema-v2. due_date→due_at, is_published→status
// (publish_status), file_path→file_key. Per-batch / per-course / per-creator
// listing collapses into ListAssignments with optional filters.
package assignments

import (
	"context"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

func statusFor(published bool) db.PublishStatus {
	if published {
		return db.PublishStatusPublished
	}
	return db.PublishStatusDraft
}

func nnum(f float64) pgtype.Numeric {
	if f == 0 {
		return pgtype.Numeric{}
	}
	return utils.NumericFromFloat(f)
}

func ntext(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func nts(t *time.Time) pgtype.Timestamptz {
	if t == nil || t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

type CreateAssignmentRequest struct {
	BatchID       *uuid.UUID `json:"batch_id"`
	CourseID      *uuid.UUID `json:"course_id"`
	LessonID      *uuid.UUID `json:"lesson_id"`
	ChapterID     *uuid.UUID `json:"chapter_id"`
	TopicID       *uuid.UUID `json:"topic_id"`
	Title         string     `json:"title" validate:"required,min=3"`
	Description   string     `json:"description"`
	AttachmentURL string     `json:"attachment_url"`
	DueDate       *time.Time `json:"due_date"`
	DueAt         *time.Time `json:"due_at"`
	MaxMarks      float64    `json:"max_marks"`
	IsPublished   bool       `json:"is_published"`
}

func (r CreateAssignmentRequest) due() *time.Time {
	if r.DueAt != nil {
		return r.DueAt
	}
	return r.DueDate
}

func (s *Service) Create(ctx context.Context, tenantID, creator uuid.UUID, req CreateAssignmentRequest) (db.CreateAssignmentRow, error) {
	return s.q.CreateAssignment(ctx, db.CreateAssignmentParams{
		TenantID:      utils.UUIDToPg(tenantID),
		Title:         req.Title,
		CourseID:      utils.UUIDPtrToPg(req.CourseID),
		BatchID:       utils.UUIDPtrToPg(req.BatchID),
		LessonID:      utils.UUIDPtrToPg(req.LessonID),
		ChapterID:     utils.UUIDPtrToPg(req.ChapterID),
		TopicID:       utils.UUIDPtrToPg(req.TopicID),
		Description:   ntext(req.Description),
		AttachmentUrl: ntext(req.AttachmentURL),
		DueAt:         nts(req.due()),
		MaxMarks:      nnum(req.MaxMarks),
		Status:        db.NullPublishStatus{PublishStatus: statusFor(req.IsPublished), Valid: true},
		CreatedBy:     utils.UUIDToPg(creator),
	})
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.GetAssignmentRow, error) {
	return s.q.GetAssignment(ctx, utils.UUIDToPg(id))
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID, courseID, batchID, createdBy *uuid.UUID, limit, offset int32) ([]db.ListAssignmentsRow, error) {
	return s.q.ListAssignments(ctx, db.ListAssignmentsParams{
		TenantID:  utils.UUIDToPg(tenantID),
		CourseID:  utils.UUIDPtrToPg(courseID),
		BatchID:   utils.UUIDPtrToPg(batchID),
		CreatedBy: utils.UUIDPtrToPg(createdBy),
		Limit:     limit, Offset: offset,
	})
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req CreateAssignmentRequest) (db.UpdateAssignmentRow, error) {
	return s.q.UpdateAssignment(ctx, db.UpdateAssignmentParams{
		ID:            utils.UUIDToPg(id),
		Title:         ntext(req.Title),
		Description:   ntext(req.Description),
		AttachmentUrl: ntext(req.AttachmentURL),
		DueAt:         nts(req.due()),
		MaxMarks:      nnum(req.MaxMarks),
		Status:        db.NullPublishStatus{PublishStatus: statusFor(req.IsPublished), Valid: true},
	})
}

func (s *Service) SetPublished(ctx context.Context, id uuid.UUID, published bool) error {
	return s.q.SetAssignmentStatus(ctx, db.SetAssignmentStatusParams{
		ID: utils.UUIDToPg(id), Status: statusFor(published),
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteAssignment(ctx, utils.UUIDToPg(id))
}

type SubmitRequest struct {
	AssignmentID   uuid.UUID `json:"assignment_id" validate:"required"`
	SubmissionText string    `json:"submission_text"`
	Content        string    `json:"content"`   // legacy alias
	FileKey        string    `json:"file_key"`
	FileURL        string    `json:"file_url"`  // legacy alias
}

func (r SubmitRequest) text() string {
	if r.SubmissionText != "" {
		return r.SubmissionText
	}
	return r.Content
}
func (r SubmitRequest) file() string {
	if r.FileKey != "" {
		return r.FileKey
	}
	return r.FileURL
}

func (s *Service) Submit(ctx context.Context, tenantID, userID uuid.UUID, req SubmitRequest) (db.SubmitAssignmentRow, error) {
	return s.q.SubmitAssignment(ctx, db.SubmitAssignmentParams{
		TenantID:       utils.UUIDToPg(tenantID),
		AssignmentID:   utils.UUIDToPg(req.AssignmentID),
		UserID:         utils.UUIDToPg(userID),
		SubmissionText: ntext(req.text()),
		FileKey:        ntext(req.file()),
	})
}

func (s *Service) GetMySubmission(ctx context.Context, userID, assignmentID uuid.UUID) (db.GetMySubmissionRow, error) {
	return s.q.GetMySubmission(ctx, db.GetMySubmissionParams{
		AssignmentID: utils.UUIDToPg(assignmentID), UserID: utils.UUIDToPg(userID),
	})
}

func (s *Service) ListSubmissions(ctx context.Context, assignmentID uuid.UUID) ([]db.ListSubmissionsRow, error) {
	return s.q.ListSubmissions(ctx, utils.UUIDToPg(assignmentID))
}

func (s *Service) ListMySubmissions(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]db.ListMySubmissionsRow, error) {
	return s.q.ListMySubmissions(ctx, db.ListMySubmissionsParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
		Limit: limit, Offset: offset,
	})
}

type GradeRequest struct {
	MarksObtained float64 `json:"marks_obtained"`
	Feedback      string  `json:"feedback"`
}

func (s *Service) Grade(ctx context.Context, graderID, submissionID uuid.UUID, req GradeRequest) (db.GradeSubmissionRow, error) {
	return s.q.GradeSubmission(ctx, db.GradeSubmissionParams{
		ID:            utils.UUIDToPg(submissionID),
		MarksObtained: utils.NumericFromFloat(req.MarksObtained),
		Feedback:      ntext(req.Feedback),
		GradedBy:      utils.UUIDToPg(graderID),
	})
}
