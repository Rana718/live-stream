package users

import (
	"context"
	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/pgtype"
)

type Service struct {
	queries *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		queries: db.New(pool),
	}
}

func (s *Service) GetUserByID(ctx context.Context, userID uuid.UUID) (*db.User, error) {
	pgUUID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.queries.GetUserByID(ctx, pgUUID)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) GetUserProfile(ctx context.Context, userID uuid.UUID) (*UserProfile, error) {
	pgUUID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.queries.GetUserByID(ctx, pgUUID)
	if err != nil {
		return nil, err
	}

	role := "student"
	if user.Role.Valid {
		role = user.Role.String
	}

	fullName := ""
	if user.FullName.Valid {
		fullName = user.FullName.String
	}

	email := ""
	if user.Email.Valid {
		email = user.Email.String
	}
	phone := ""
	if user.PhoneNumber.Valid {
		phone = user.PhoneNumber.String
	}

	return &UserProfile{
		ID:                  uuid.UUID(user.ID.Bytes),
		Email:               email,
		Phone:               phone,
		FullName:            fullName,
		Role:                role,
		IsActive:            user.IsActive.Bool,
		ClassLevel:          textPtr(user.ClassLevel),
		Board:               textPtr(user.Board),
		ExamGoal:            textPtr(user.ExamGoal),
		OnboardingCompleted: user.OnboardingCompleted.Bool,
	}, nil
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func (s *Service) UpdateUser(ctx context.Context, userID uuid.UUID, fullName string) (*db.User, error) {
	pgUUID := pgtype.UUID{Bytes: userID, Valid: true}
	pgFullName := pgtype.Text{String: fullName, Valid: true}

	user, err := s.queries.UpdateUser(ctx, db.UpdateUserParams{
		ID:       pgUUID,
		FullName: pgFullName,
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// OnboardingInput mirrors the fields collected by the mobile onboarding flow.
// Any field may be empty — class_level / exam_goal are allowed to be null so
// a learner can opt out of a dimension (e.g. a pure competitive-exam student
// without a school class).
type OnboardingInput struct {
	FullName   string
	ClassLevel string
	Board      string
	ExamGoal   string
}

func (s *Service) CompleteOnboarding(ctx context.Context, userID uuid.UUID, in OnboardingInput) (*db.User, error) {
	pgUUID := pgtype.UUID{Bytes: userID, Valid: true}

	user, err := s.queries.UpdateOnboardingProfile(ctx, db.UpdateOnboardingProfileParams{
		ID:         pgUUID,
		Column2:    in.FullName,
		ClassLevel: textOrNull(in.ClassLevel),
		Board:      textOrNull(in.Board),
		ExamGoal:   textOrNull(in.ExamGoal),
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func textOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: s, Valid: true}
}

func (s *Service) ListUsers(ctx context.Context, limit, offset int32) ([]db.User, error) {
	return s.queries.ListUsers(ctx, db.ListUsersParams{
		Limit:  limit,
		Offset: offset,
	})
}

type UserProfile struct {
	ID                  uuid.UUID `json:"id"`
	Phone               string    `json:"phone"`
	Email               string    `json:"email,omitempty"`
	FullName            string    `json:"full_name"`
	Role                string    `json:"role"`
	IsActive            bool      `json:"is_active"`
	ClassLevel          *string   `json:"class_level"`
	Board               *string   `json:"board"`
	ExamGoal            *string   `json:"exam_goal"`
	OnboardingCompleted bool      `json:"onboarding_completed"`
}
