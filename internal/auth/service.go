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

// SMSClient is the minimal contract auth needs for OTP delivery. The default
// wiring is the MSG91 implementation under internal/sms; in dev we leave it
// nil and devModeOTP short-circuits the send.
type SMSClient interface {
	SendOTP(ctx context.Context, phone, code string) error
}

// Referrer is the slice of internal/referrals that auth depends on. We
// inject it (rather than import the package) so the dependency is one-way
// and testable. Best-effort: an invalid referral code never fails signup.
type Referrer interface {
	AttachToSignup(ctx context.Context, tenantID, newUserID uuid.UUID, code string)
}

type Service struct {
	queries  *db.Queries
	redis    *redis.Client
	cfg      *config.Config
	sms      SMSClient
	referrer Referrer
}

func NewService(pool *pgxpool.Pool, redis *redis.Client, cfg *config.Config) *Service {
	return &Service{
		queries: db.New(pool),
		redis:   redis,
		cfg:     cfg,
	}
}

// WithSMS wires an SMS client. Optional — production sets it via main.go,
// tests can leave it nil.
func (s *Service) WithSMS(c SMSClient) *Service { s.sms = c; return s }

// WithReferrer wires the referrals service so OTP verify can attach a
// referral code to a fresh signup. Optional — leaving nil disables
// referral tracking without breaking anything else.
func (s *Service) WithReferrer(r Referrer) *Service { s.referrer = r; return s }

// Email + password registration was removed in favor of phone-OTP and
// Google sign-in only. The RegisterRequest type is kept here as a thin
// adapter for the legacy /auth/register/* endpoints during the deprecation
// window — those endpoints now refuse to issue tokens and only create a
// shell user record (used by automated tests that don't go through OTP).
type RegisterRequest struct {
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
	OrgCode  string `json:"org_code"`
}

type TokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	User         UserInfo `json:"user"`
}

type UserInfo struct {
	ID       uuid.UUID `json:"id"`
	Phone    string    `json:"phone"`
	Email    string    `json:"email,omitempty"`
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

// register creates a shell user record in a tenant — no password, no email,
// just phone + name. It is invoked from the legacy /auth/register/* admin
// endpoints (admins occasionally bulk-create student accounts before the
// student has logged in via OTP). The student then completes auth via
// /auth/otp/verify which finds this row by phone.
func (s *Service) register(ctx context.Context, req RegisterRequest) (*db.User, error) {
	if req.Role == "" {
		req.Role = "student"
	}

	tenantID, err := s.resolveTenant(ctx, req.OrgCode)
	if err != nil {
		return nil, err
	}

	user, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		TenantID:     pgtype.UUID{Bytes: tenantID, Valid: true},
		PhoneNumber:  pgtype.Text{String: req.Phone, Valid: req.Phone != ""},
		Email:        pgtype.Text{}, // email is now optional
		PasswordHash: pgtype.Text{}, // no password — phone OTP / Google only
		FullName:     pgtype.Text{String: req.FullName, Valid: req.FullName != ""},
		Role:         pgtype.Text{String: req.Role, Valid: true},
		AuthMethod:   pgtype.Text{String: "phone", Valid: true},
	})
	if err != nil {
		return nil, err
	}

	_, _ = s.queries.AddTenantUser(ctx, db.AddTenantUserParams{
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
		UserID:   user.ID,
		Role:     req.Role,
	})

	return &user, nil
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

	// Email is now nullable; use it for the JWT only when present.
	emailClaim := ""
	if user.Email.Valid {
		emailClaim = user.Email.String
	}
	phone := ""
	if user.PhoneNumber.Valid {
		phone = user.PhoneNumber.String
	}

	accessToken, err := utils.GenerateAccessToken(userID, emailClaim, role, tenantID, s.cfg.JWT.AccessSecret, accessExpiry)
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
			Email:    emailClaim,
			Phone:    phone,
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
	Phone               string    `json:"phone"`
	Email               string    `json:"email,omitempty"`
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
		Role:                "student",
		OnboardingCompleted: user.OnboardingCompleted.Bool,
	}
	if user.Email.Valid {
		me.Email = user.Email.String
	}
	if user.PhoneNumber.Valid {
		me.Phone = user.PhoneNumber.String
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

	emailClaim := ""
	if user.Email.Valid {
		emailClaim = user.Email.String
	}
	phone := ""
	if user.PhoneNumber.Valid {
		phone = user.PhoneNumber.String
	}

	newAccessToken, err := utils.GenerateAccessToken(userID, emailClaim, role, tenantID, s.cfg.JWT.AccessSecret, accessExpiry)
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
			Email:    emailClaim,
			Phone:    phone,
			FullName: fullName,
			Role:     role,
			TenantID: tenantID,
		},
	}, nil
}
