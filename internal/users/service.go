// Package users serves the authenticated user's own profile (global `users`
// row + per-tenant `user_profiles`) and the tenant-admin member roster.
package users

import (
	"context"

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

// Profile is the merged view returned by GET /users/profile.
type Profile struct {
	ID                  uuid.UUID `json:"id"`
	Email               string    `json:"email,omitempty"`
	Phone               string    `json:"phone,omitempty"`
	FullName            string    `json:"full_name"`
	AvatarURL           string    `json:"avatar_url,omitempty"`
	Role                string    `json:"role"`
	TenantID            uuid.UUID `json:"tenant_id"`
	ClassLevel          *string   `json:"class_level"`
	Board               *string   `json:"board"`
	ExamGoal            *string   `json:"exam_goal"`
	GuardianName        *string   `json:"guardian_name"`
	GuardianPhone       *string   `json:"guardian_phone"`
	OnboardingCompleted bool      `json:"onboarding_completed"`
}

func (s *Service) GetProfile(ctx context.Context, tenantID, userID uuid.UUID, role string) (*Profile, error) {
	u, err := s.q.GetUserByID(ctx, utils.UUIDToPg(userID))
	if err != nil {
		return nil, err
	}
	p := &Profile{
		ID: userID, TenantID: tenantID, Role: role,
		Email: utils.TextFromPg(u.Email), Phone: utils.TextFromPg(u.Phone),
		FullName: utils.TextFromPg(u.FullName), AvatarURL: utils.TextFromPg(u.AvatarUrl),
	}
	if prof, err := s.q.GetUserProfile(ctx, db.GetUserProfileParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	}); err == nil {
		p.OnboardingCompleted = prof.OnboardingCompleted
		p.ClassLevel = textPtr(prof.ClassLevel)
		p.Board = textPtr(prof.Board)
		p.ExamGoal = textPtr(prof.ExamGoal)
		p.GuardianName = textPtr(prof.GuardianName)
		p.GuardianPhone = textPtr(prof.GuardianPhone)
	}
	return p, nil
}

// UpdateBasics changes the global user's name / avatar.
func (s *Service) UpdateBasics(ctx context.Context, userID uuid.UUID, fullName, avatarURL string) error {
	_, err := s.q.UpdateUserProfileFields(ctx, db.UpdateUserProfileFieldsParams{
		ID:        utils.UUIDToPg(userID),
		FullName:  utils.TextToPg(fullName),
		AvatarUrl: utils.TextToPg(avatarURL),
	})
	return err
}

type OnboardingInput struct {
	FullName      string
	ClassLevel    string
	Board         string
	ExamGoal      string
	GuardianName  string
	GuardianPhone string
}

func (s *Service) CompleteOnboarding(ctx context.Context, tenantID, userID uuid.UUID, in OnboardingInput) error {
	if in.FullName != "" {
		if _, err := s.q.UpdateUserProfileFields(ctx, db.UpdateUserProfileFieldsParams{
			ID: utils.UUIDToPg(userID), FullName: utils.TextToPg(in.FullName),
		}); err != nil {
			return err
		}
	}
	_, err := s.q.UpsertUserProfile(ctx, db.UpsertUserProfileParams{
		TenantID:            utils.UUIDToPg(tenantID),
		UserID:              utils.UUIDToPg(userID),
		ClassLevel:          utils.TextToPg(in.ClassLevel),
		Board:               utils.TextToPg(in.Board),
		ExamGoal:            utils.TextToPg(in.ExamGoal),
		GuardianName:        utils.TextToPg(in.GuardianName),
		GuardianPhone:       utils.TextToPg(in.GuardianPhone),
		OnboardingCompleted: pgtype.Bool{Bool: true, Valid: true},
	})
	return err
}

// ListMembers is the tenant-admin roster.
func (s *Service) ListMembers(ctx context.Context, tenantID uuid.UUID, role string, limit, offset int32) ([]db.ListTenantMembersRow, error) {
	var rolePtr db.NullTenantRole
	if role != "" {
		rolePtr = db.NullTenantRole{TenantRole: db.TenantRole(role), Valid: true}
	}
	return s.q.ListTenantMembers(ctx, db.ListTenantMembersParams{
		TenantID: utils.UUIDToPg(tenantID),
		Role:     rolePtr,
		Limit:    limit,
		Offset:   offset,
	})
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid || t.String == "" {
		return nil
	}
	v := t.String
	return &v
}
