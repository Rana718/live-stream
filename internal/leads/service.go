// Package leads handles inbound interest from the marketing site. Public
// POST /public/leads stores whatever a prospect filled in; super-admin
// endpoints triage from there.
package leads

import (
	"context"

	"live-platform/internal/database"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type CreateLeadInput struct {
	Name          string `json:"name" validate:"required,min=2,max=200"`
	Phone         string `json:"phone" validate:"required,min=7,max=20"`
	Email         string `json:"email"`
	InstituteName string `json:"institute_name"`
	City          string `json:"city"`
	StudentsCount int    `json:"students_count"`
	Source        string `json:"source"`
}

func (s *Service) Create(ctx context.Context, in CreateLeadInput) (db.CreateLeadRow, error) {
	// leads is platform-level (nullable tenant_id) with a public-INSERT policy
	// but no non-super SELECT policy — so INSERT ... RETURNING needs the
	// super-admin scope to read the row back. No tenant data is involved.
	ctx = database.WithSuperAdmin(ctx)
	return s.q.CreateLead(ctx, db.CreateLeadParams{
		Name:          utils.TextToPg(in.Name),
		Phone:         utils.TextToPg(in.Phone),
		Email:         utils.TextToPg(in.Email),
		InstituteName: utils.TextToPg(in.InstituteName),
		City:          utils.TextToPg(in.City),
		StudentsCount: utils.Int4ToPg(int32(in.StudentsCount)),
		Source:        utils.TextToPg(in.Source),
	})
}

func (s *Service) List(ctx context.Context, status string, limit, offset int32) ([]db.ListLeadsRow, error) {
	var st db.NullLeadStatus
	if status != "" {
		st = db.NullLeadStatus{LeadStatus: db.LeadStatus(status), Valid: true}
	}
	return s.q.ListLeads(database.WithSuperAdmin(ctx), db.ListLeadsParams{
		Status: st, Limit: limit, Offset: offset,
	})
}

func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, status string, assignedTo uuid.UUID, notes string) error {
	return s.q.UpdateLeadStatus(database.WithSuperAdmin(ctx), db.UpdateLeadStatusParams{
		ID:         utils.UUIDToPg(id),
		Status:     db.LeadStatus(status),
		AssignedTo: pgtype.UUID{Bytes: assignedTo, Valid: assignedTo != uuid.Nil},
		Notes:      utils.TextToPg(notes),
	})
}
