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

	return &UserProfile{
		ID:       uuid.UUID(user.ID.Bytes),
		Email:    user.Email,
		Username: user.Username,
		FullName: fullName,
		Role:     role,
		IsActive: user.IsActive.Bool,
	}, nil
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

func (s *Service) ListUsers(ctx context.Context, limit, offset int32) ([]db.User, error) {
	return s.queries.ListUsers(ctx, db.ListUsersParams{
		Limit:  limit,
		Offset: offset,
	})
}

type UserProfile struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	Username string    `json:"username"`
	FullName string    `json:"full_name"`
	Role     string    `json:"role"`
	IsActive bool      `json:"is_active"`
}
