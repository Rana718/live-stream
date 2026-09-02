// Package batches — schema-v2. start_date/end_date → starts_on/ends_on
// (date). No current_students counter (derive from enrollments when needed).
package batches

import (
	"context"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

func dptr(t *time.Time) pgtype.Date {
	if t == nil || t.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}
func ntext(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

type CreateBatchRequest struct {
	CourseID     uuid.UUID  `json:"course_id" validate:"required"`
	Name         string     `json:"name" validate:"required"`
	Description  string     `json:"description"`
	InstructorID *uuid.UUID `json:"instructor_id"`
	StartsOn     *time.Time `json:"starts_on"`
	StartDate    *time.Time `json:"start_date"` // legacy alias
	EndsOn       *time.Time `json:"ends_on"`
	EndDate      *time.Time `json:"end_date"` // legacy alias
	MaxStudents  int32      `json:"max_students"`
	IsActive     bool       `json:"is_active"`
}

func (r CreateBatchRequest) starts() *time.Time {
	if r.StartsOn != nil {
		return r.StartsOn
	}
	return r.StartDate
}
func (r CreateBatchRequest) ends() *time.Time {
	if r.EndsOn != nil {
		return r.EndsOn
	}
	return r.EndDate
}

type Batch struct {
	ID           string     `json:"id"`
	CourseID     string     `json:"course_id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	InstructorID string     `json:"instructor_id"`
	StartsOn     *time.Time `json:"starts_on"`
	StartDate    *time.Time `json:"start_date"`
	EndsOn       *time.Time `json:"ends_on"`
	EndDate      *time.Time `json:"end_date"`
	MaxStudents  int32      `json:"max_students"`
	IsActive     bool       `json:"is_active"`
}

func dval(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

func mk(id, courseID pgtype.UUID, name string, desc pgtype.Text, instr pgtype.UUID, s, e pgtype.Date, max pgtype.Int4, active bool) Batch {
	st, en := dval(s), dval(e)
	return Batch{
		ID: utils.UUIDFromPg(id), CourseID: utils.UUIDFromPg(courseID), Name: name,
		Description: utils.TextFromPg(desc), InstructorID: utils.UUIDFromPg(instr),
		StartsOn: st, StartDate: st, EndsOn: en, EndDate: en,
		MaxStudents: utils.Int4FromPg(max), IsActive: active,
	}
}

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, req CreateBatchRequest) (Batch, error) {
	row, err := s.q.CreateBatch(ctx, db.CreateBatchParams{
		TenantID:     utils.UUIDToPg(tenantID),
		CourseID:     utils.UUIDToPg(req.CourseID),
		Name:         req.Name,
		Description:  ntext(req.Description),
		InstructorID: utils.UUIDPtrToPg(req.InstructorID),
		StartsOn:     dptr(req.starts()),
		EndsOn:       dptr(req.ends()),
		MaxStudents:  utils.Int4ToPg(req.MaxStudents),
	})
	if err != nil {
		return Batch{}, err
	}
	return mk(row.ID, row.CourseID, row.Name, pgtype.Text{}, row.InstructorID, row.StartsOn, row.EndsOn, row.MaxStudents, row.IsActive), nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Batch, error) {
	r, err := s.q.GetBatch(ctx, utils.UUIDToPg(id))
	if err != nil {
		return Batch{}, err
	}
	return mk(r.ID, r.CourseID, r.Name, r.Description, r.InstructorID, r.StartsOn, r.EndsOn, r.MaxStudents, r.IsActive), nil
}

func (s *Service) ListByCourse(ctx context.Context, tenantID, courseID uuid.UUID) ([]Batch, error) {
	rows, err := s.q.ListBatchesByCourse(ctx, db.ListBatchesByCourseParams{
		TenantID: utils.UUIDToPg(tenantID), CourseID: utils.UUIDToPg(courseID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Batch, 0, len(rows))
	for _, r := range rows {
		b := mk(r.ID, utils.UUIDToPg(courseID), r.Name, r.Description, r.InstructorID, r.StartsOn, r.EndsOn, r.MaxStudents, r.IsActive)
		out = append(out, b)
	}
	return out, nil
}

func (s *Service) ListForTenant(ctx context.Context, tenantID uuid.UUID, instructorID *uuid.UUID, limit, offset int32) ([]Batch, error) {
	rows, err := s.q.ListBatchesForTenant(ctx, db.ListBatchesForTenantParams{
		TenantID: utils.UUIDToPg(tenantID), InstructorID: utils.UUIDPtrToPg(instructorID),
		Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Batch, 0, len(rows))
	for _, r := range rows {
		out = append(out, mk(r.ID, r.CourseID, r.Name, r.Description, r.InstructorID, r.StartsOn, r.EndsOn, r.MaxStudents, r.IsActive))
	}
	return out, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req CreateBatchRequest) (Batch, error) {
	r, err := s.q.UpdateBatch(ctx, db.UpdateBatchParams{
		ID:           utils.UUIDToPg(id),
		Name:         ntext(req.Name),
		Description:  ntext(req.Description),
		InstructorID: utils.UUIDPtrToPg(req.InstructorID),
		StartsOn:     dptr(req.starts()),
		EndsOn:       dptr(req.ends()),
		MaxStudents:  utils.Int4ToPg(req.MaxStudents),
		IsActive:     pgtype.Bool{Bool: req.IsActive, Valid: true},
	})
	if err != nil {
		return Batch{}, err
	}
	return mk(r.ID, pgtype.UUID{}, r.Name, pgtype.Text{}, r.InstructorID, r.StartsOn, r.EndsOn, r.MaxStudents, r.IsActive), nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteBatch(ctx, utils.UUIDToPg(id))
}
