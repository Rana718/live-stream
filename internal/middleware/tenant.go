package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"live-platform/internal/database"
)

// TenantContext attaches the current request's tenant/user IDs to the Go
// context so every DB query made downstream is correctly RLS-scoped. Must
// run AFTER AuthMiddleware so the JWT-derived tenant_id is available in
// c.Locals.
//
// This doesn't touch the database directly — every service in this
// codebase holds a db.Queries built once at startup from the shared
// *pgxpool.Pool, so there's no single "this request's connection" to set
// session GUCs on. Instead, database.WithTenant tags the context, and
// NewPostgresPool's BeforeAcquire hook (internal/database/postgres.go)
// applies the actual `SET_CONFIG` on whichever physical connection ends
// up serving each query made with it — see the comment there for the
// full reasoning, including why an earlier version of this middleware
// that eagerly acquired+configured one connection per request was
// actually a no-op (nothing downstream ever used that connection).
//
// For super-admin endpoints, mount SuperAdminContext instead.
func TenantContext() fiber.Handler {
	return func(c fiber.Ctx) error {
		tenantID, ok := c.Locals("tenantID").(uuid.UUID)
		if !ok || tenantID == uuid.Nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing tenant context",
			})
		}
		userID, _ := c.Locals("userID").(uuid.UUID)

		c.SetContext(database.WithTenant(c.Context(), tenantID.String(), userID.String()))
		return c.Next()
	}
}

// SuperAdminContext is a convenience wrapper for cron / provisioning
// endpoints that need to bypass RLS. Mount this on the platform-admin route
// group instead of TenantContext.
func SuperAdminContext() fiber.Handler {
	return func(c fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if role != "super_admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "super_admin only",
			})
		}

		c.SetContext(database.WithSuperAdmin(c.Context()))
		return c.Next()
	}
}

// PublicLookupContext is mounted on /public/tenants/by-code/:code. It opts
// the request into the tenant_public_orgcode RLS policy so an unauthenticated
// browser can resolve an Org Code to a tenant for branding before login.
func PublicLookupContext() fiber.Handler {
	return func(c fiber.Ctx) error {
		c.SetContext(database.WithPublicLookup(c.Context()))
		return c.Next()
	}
}
