// Package leads handles inbound interest from the marketing website. The
// public POST /public/leads endpoint takes whatever a prospect filled in
// and stores it; super_admin endpoints triage from there.
package leads

import (
	"context"

	"live-platform/internal/database/db"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type CreateLeadInput struct {
	Name           string `json:"name" validate:"required,min=2,max=200"`
	Phone          string `json:"phone" validate:"required,min=7,max=20"`
	Email          string `json:"email"`
	InstituteName  string `json:"institute_name"`
	City           string `json:"city"`
	StudentsCount  int    `json:"students_count"`
	Source         string `json:"source"`
	Notes          string `json:"notes"`
}

func (s *Service) Create(ctx context.Context, in CreateLeadInput) (*db.Lead, error) {
	row, err := s.q.CreateLead(ctx, db.CreateLeadParams{
		Name:          pgtype.Text{String: in.Name, Valid: in.Name != ""},
		Phone:         pgtype.Text{String: in.Phone, Valid: in.Phone != ""},
		Email:         pgtype.Text{String: in.Email, Valid: in.Email != ""},
		InstituteName: pgtype.Text{String: in.InstituteName, Valid: in.InstituteName != ""},
		City:          pgtype.Text{String: in.City, Valid: in.City != ""},
		StudentsCount: pgtype.Int4{Int32: int32(in.StudentsCount), Valid: in.StudentsCount > 0},
		Column7:       in.Source,
		Notes:         pgtype.Text{String: in.Notes, Valid: in.Notes != ""},
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) List(ctx context.Context, status string, limit, offset int32) ([]db.Lead, error) {
	return s.q.ListLeads(ctx, db.ListLeadsParams{
		Column1: status,
		Limit:   limit,
		Offset:  offset,
	})
}
