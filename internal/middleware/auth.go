package middleware

import (
	"strings"

	"live-platform/internal/config"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// Locals keys set by AuthMiddleware.
const (
	LocalUserID   = "userID"
	LocalTenantID = "tenantID"
	LocalRole     = "role"
	LocalEmail    = "email"
	LocalTokenVer = "tokenVer"
)

func bearerToken(c fiber.Ctx) string {
	h := c.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func setAuthLocals(c fiber.Ctx, claims *utils.Claims) {
	c.Locals(LocalUserID, claims.UserID)
	c.Locals(LocalTenantID, claims.TenantID)
	c.Locals(LocalRole, claims.Role)
	c.Locals(LocalEmail, claims.Email)
	c.Locals(LocalTokenVer, claims.Ver)
}

// AuthMiddleware requires a valid access token. The token_version check
// against the DB (revocation) is applied in the auth service's session
// endpoints and at refresh time; a stale access token lives at most one
// access-TTL (default 15m).
func AuthMiddleware(cfg *config.JWTConfig) fiber.Handler {
	return func(c fiber.Ctx) error {
		tok := bearerToken(c)
		if tok == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authentication token"})
		}
		claims, err := utils.ValidateToken(tok, cfg.AccessSecret)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired token"})
		}
		setAuthLocals(c, claims)
		return c.Next()
	}
}

// OptionalAuthMiddleware attaches identity when a valid token is present but
// never rejects the request.
func OptionalAuthMiddleware(cfg *config.JWTConfig) fiber.Handler {
	return func(c fiber.Ctx) error {
		if tok := bearerToken(c); tok != "" {
			if claims, err := utils.ValidateToken(tok, cfg.AccessSecret); err == nil {
				setAuthLocals(c, claims)
			}
		}
		return c.Next()
	}
}

// RequireRole allows the request only if the caller's tenant role is in the
// allowed set. A platform super_admin passes every check.
func RequireRole(allowed ...string) fiber.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	return func(c fiber.Ctx) error {
		role, _ := c.Locals(LocalRole).(string)
		if role == "super_admin" {
			return c.Next()
		}
		if _, ok := set[role]; ok {
			return c.Next()
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "insufficient permissions"})
	}
}

// Role-group helpers matching schema-v2 tenant_role values.
func OwnerOnly() fiber.Handler    { return RequireRole("owner") }
func AdminOnly() fiber.Handler    { return RequireRole("owner", "admin") }
func StaffOrAbove() fiber.Handler { return RequireRole("owner", "admin", "instructor", "staff") }
func InstructorOrAdmin() fiber.Handler {
	return RequireRole("owner", "admin", "instructor")
}
func StudentOrAbove() fiber.Handler {
	return RequireRole("owner", "admin", "instructor", "staff", "student", "parent")
}

// CurrentUserID / CurrentTenantID read the authenticated identity from
// Locals; zero UUID when absent.
func CurrentUserID(c fiber.Ctx) uuid.UUID {
	id, _ := c.Locals(LocalUserID).(uuid.UUID)
	return id
}

func CurrentTenantID(c fiber.Ctx) uuid.UUID {
	id, _ := c.Locals(LocalTenantID).(uuid.UUID)
	return id
}
