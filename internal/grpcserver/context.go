package grpcserver

import (
	"context"

	"live-platform/internal/database"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ctxKey int

const (
	ctxTenant ctxKey = iota
	ctxUser
	ctxRole
)

// caller is the authenticated identity for one RPC, populated by
// authInterceptor. Mirrors what middleware.AuthMiddleware puts in Fiber Locals.
type caller struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	Role     string
	Super    bool
}

func callerFrom(ctx context.Context) (caller, bool) {
	role, _ := ctx.Value(ctxRole).(string)
	if role == "" {
		return caller{}, false
	}
	t, _ := ctx.Value(ctxTenant).(uuid.UUID)
	u, _ := ctx.Value(ctxUser).(uuid.UUID)
	return caller{TenantID: t, UserID: u, Role: role, Super: role == "super_admin"}, true
}

// requireTenant returns the caller or Unauthenticated. A super-admin without a
// tenant is allowed (platform-scoped RPCs check c.Super themselves).
func requireTenant(ctx context.Context) (caller, error) {
	c, ok := callerFrom(ctx)
	if !ok || (c.TenantID == uuid.Nil && !c.Super) {
		return caller{}, status.Error(codes.Unauthenticated, "missing tenant context")
	}
	return c, nil
}

// permDenied is the canonical "not allowed" status for inline platform-scope checks.
var permDenied = status.Error(codes.PermissionDenied, "insufficient permissions")

// require enforces a role allowlist (super_admin always passes).
func (c caller) require(roles ...string) error {
	if c.Super {
		return nil
	}
	for _, r := range roles {
		if r == c.Role {
			return nil
		}
	}
	return status.Error(codes.PermissionDenied, "insufficient permissions")
}

// Role allowlists matching internal/middleware/auth.go helpers.
var (
	rolesAdminOnly    = []string{"owner", "admin"}
	rolesInstructorUp = []string{"owner", "admin", "instructor"}
	rolesStaffUp      = []string{"owner", "admin", "instructor", "staff"}
	rolesStudentUp    = []string{"owner", "admin", "instructor", "staff", "student", "parent"}
)

// WithTestIdentity stamps an authenticated identity + RLS scope onto ctx
// without minting a JWT. Test-only: it lets adapter tests call a server
// registered without authInterceptor. Not used by production code.
func WithTestIdentity(ctx context.Context, tenantID, userID uuid.UUID, role string) context.Context {
	ctx = context.WithValue(ctx, ctxTenant, tenantID)
	ctx = context.WithValue(ctx, ctxUser, userID)
	ctx = context.WithValue(ctx, ctxRole, role)
	if role == "super_admin" {
		return database.WithSuperAdmin(ctx)
	}
	return database.WithTenant(ctx, tenantID.String(), userID.String())
}

func parseUUID(s, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Errorf(codes.InvalidArgument, "%s must be a uuid", field)
	}
	return id, nil
}

// optUUID parses an optional uuid field; "" → nil, invalid → error.
func optUUID(s, field string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s must be a uuid", field)
	}
	return &id, nil
}
