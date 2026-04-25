package auth

import (
	"context"
	"fmt"
	"live-platform/internal/config"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Service struct {
	queries *db.Queries
	redis   *redis.Client
	cfg     *config.Config
}

func NewService(pool *pgxpool.Pool, redis *redis.Client, cfg *config.Config) *Service {
	return &Service{
		queries: db.New(pool),
		redis:   redis,
		cfg:     cfg,
	}
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
	OrgCode  string `json:"org_code"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgCode  string `json:"org_code"`
}

type TokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	User         UserInfo `json:"user"`
}

type UserInfo struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	Username string    `json:"username"`
	FullName string    `json:"full_name"`
	Role     string    `json:"role"`
	TenantID uuid.UUID `json:"tenant_id"`
}

// DefaultTenantID is the seed tenant used pre-migration and during dev when
// no Org Code is provided. Production deployments should require Org Code.
var DefaultTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// resolveTenant turns an Org Code (case-insensitive) into a tenant UUID. If
// the Org Code is blank we fall back to the default tenant — useful during
// the migration window before every client passes Org Code, but lock this
// down once all clients are updated.
func (s *Service) resolveTenant(ctx context.Context, orgCode string) (uuid.UUID, error) {
	if orgCode == "" {
		return DefaultTenantID, nil
	}
	t, err := s.queries.GetTenantByOrgCode(ctx, orgCode)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid org code")
	}
	return uuid.UUID(t.ID.Bytes), nil
}

func (s *Service) RegisterStudent(ctx context.Context, req RegisterRequest) (*db.User, error) {
	req.Role = "student"
	return s.register(ctx, req)
}

func (s *Service) RegisterInstructor(ctx context.Context, req RegisterRequest) (*db.User, error) {
	req.Role = "instructor"
	return s.register(ctx, req)
}

func (s *Service) RegisterAdmin(ctx context.Context, req RegisterRequest) (*db.User, error) {
	req.Role = "admin"
	return s.register(ctx, req)
}

func (s *Service) register(ctx context.Context, req RegisterRequest) (*db.User, error) {
	if req.Role == "" {
		req.Role = "student"
	}

	tenantID, err := s.resolveTenant(ctx, req.OrgCode)
	if err != nil {
		return nil, err
	}

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	// CreateUser is sqlc-generated. After sqlc regenerates against the new
	// migration, CreateUserParams will gain a TenantID field.
	user, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        req.Email,
		Username:     req.Username,
		PasswordHash: hash,
		FullName:     pgtype.Text{String: req.FullName, Valid: true},
		Role:         pgtype.Text{String: req.Role, Valid: true},
		TenantID:     pgtype.UUID{Bytes: tenantID, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	// Mirror the membership row so the user can resolve back to this tenant
	// at next login (and so multi-tenant users see the org in their list).
	_, _ = s.queries.AddTenantUser(ctx, db.AddTenantUserParams{
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
		UserID:   user.ID,
		Role:     req.Role,
	})

	return &user, nil
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (*TokenResponse, error) {
	tenantID, err := s.resolveTenant(ctx, req.OrgCode)
	if err != nil {
		return nil, err
	}

	user, err := s.queries.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if !user.IsActive.Bool {
		return nil, fmt.Errorf("account is inactive")
	}

	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Reject if the user doesn't belong to this Org Code. Stops a Tenant A
	// student from logging into Tenant B with stolen creds.
	if uuid.UUID(user.TenantID.Bytes) != tenantID {
		_, mErr := s.queries.GetTenantUser(ctx, db.GetTenantUserParams{
			TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
			UserID:   user.ID,
		})
		if mErr != nil {
			return nil, fmt.Errorf("invalid credentials")
		}
	}

	return s.issueTokensForUser(ctx, &user, tenantID)
}

// issueTokensForUser mints fresh access + refresh tokens for an authenticated
// user row and persists the refresh token in Redis, matching the behaviour of
// the email/password Login path. All alternative login methods (OTP, Google,
// account linking) go through this helper so refresh-token rotation stays
// consistent across login surfaces.
//
// tenantID is the resolved Org Code → tenant mapping used to scope the JWT.
// Pass uuid.Nil to use the user's primary tenant on the row.
func (s *Service) issueTokensForUser(ctx context.Context, user *db.User, tenantID uuid.UUID) (*TokenResponse, error) {
	accessExpiry, _ := time.ParseDuration(s.cfg.JWT.AccessExpiry)
	refreshExpiry, _ := time.ParseDuration(s.cfg.JWT.RefreshExpiry)

	role := "student"
	if user.Role.Valid {
		role = user.Role.String
	}

	userID := uuid.UUID(user.ID.Bytes)
	if tenantID == uuid.Nil {
		tenantID = uuid.UUID(user.TenantID.Bytes)
		if tenantID == uuid.Nil {
			tenantID = DefaultTenantID
		}
	}

	accessToken, err := utils.GenerateAccessToken(userID, user.Email, role, tenantID, s.cfg.JWT.AccessSecret, accessExpiry)
	if err != nil {
		return nil, err
	}

	refreshToken, err := utils.GenerateRefreshToken(userID, s.cfg.JWT.RefreshSecret, refreshExpiry)
	if err != nil {
		return nil, err
	}

	if err := s.redis.Set(ctx, fmt.Sprintf("refresh:%s", userID.String()), refreshToken, refreshExpiry).Err(); err != nil {
		return nil, err
	}

	fullName := ""
	if user.FullName.Valid {
		fullName = user.FullName.String
	}

	return &TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: UserInfo{
			ID:       userID,
			Email:    user.Email,
			Username: user.Username,
			FullName: fullName,
			Role:     role,
			TenantID: tenantID,
		},
	}, nil
}

func (s *Service) Logout(ctx context.Context, userID uuid.UUID) error {
	return s.redis.Del(ctx, fmt.Sprintf("refresh:%s", userID.String())).Err()
}

// MeResponse is the shape returned by GET /auth/me. The mobile app uses this
// both to rehydrate the session and to decide whether to force the user into
// the onboarding flow.
type MeResponse struct {
	ID                  uuid.UUID `json:"id"`
	Email               string    `json:"email"`
	Username            string    `json:"username"`
	FullName            string    `json:"full_name"`
	Role                string    `json:"role"`
	ClassLevel          *string   `json:"class_level"`
	Board               *string   `json:"board"`
	ExamGoal            *string   `json:"exam_goal"`
	OnboardingCompleted bool      `json:"onboarding_completed"`
}

func (s *Service) GetMe(ctx context.Context, userID uuid.UUID) (*MeResponse, error) {
	pgUUID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.queries.GetUserByID(ctx, pgUUID)
	if err != nil {
		return nil, err
	}

	me := &MeResponse{
		ID:                  uuid.UUID(user.ID.Bytes),
		Email:               user.Email,
		Username:            user.Username,
		Role:                "student",
		OnboardingCompleted: user.OnboardingCompleted.Bool,
	}
	if user.FullName.Valid {
		me.FullName = user.FullName.String
	}
	if user.Role.Valid {
		me.Role = user.Role.String
	}
	if user.ClassLevel.Valid {
		v := user.ClassLevel.String
		me.ClassLevel = &v
	}
	if user.Board.Valid {
		v := user.Board.String
		me.Board = &v
	}
	if user.ExamGoal.Valid {
		v := user.ExamGoal.String
		me.ExamGoal = &v
	}
	return me, nil
}

func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	claims, err := utils.ValidateRefreshToken(refreshToken, s.cfg.JWT.RefreshSecret)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token")
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("invalid user id")
	}

	storedToken, err := s.redis.Get(ctx, fmt.Sprintf("refresh:%s", userID.String())).Result()
	if err != nil || storedToken != refreshToken {
		return nil, fmt.Errorf("invalid refresh token")
	}

	pgUUID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.queries.GetUserByID(ctx, pgUUID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	accessExpiry, _ := time.ParseDuration(s.cfg.JWT.AccessExpiry)
	refreshExpiry, _ := time.ParseDuration(s.cfg.JWT.RefreshExpiry)

	role := "student"
	if user.Role.Valid {
		role = user.Role.String
	}

	tenantID := uuid.UUID(user.TenantID.Bytes)
	if tenantID == uuid.Nil {
		tenantID = DefaultTenantID
	}

	newAccessToken, err := utils.GenerateAccessToken(userID, user.Email, role, tenantID, s.cfg.JWT.AccessSecret, accessExpiry)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := utils.GenerateRefreshToken(userID, s.cfg.JWT.RefreshSecret, refreshExpiry)
	if err != nil {
		return nil, err
	}

	err = s.redis.Set(ctx, fmt.Sprintf("refresh:%s", userID.String()), newRefreshToken, refreshExpiry).Err()
	if err != nil {
		return nil, err
	}

	fullName := ""
	if user.FullName.Valid {
		fullName = user.FullName.String
	}

	return &TokenResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		User: UserInfo{
			ID:       userID,
			Email:    user.Email,
			Username: user.Username,
			FullName: fullName,
			Role:     role,
			TenantID: tenantID,
		},
	}, nil
}
