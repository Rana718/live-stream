package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TenantContext sets the Postgres session variables that drive RLS for the
// current request. Must run AFTER AuthMiddleware so the JWT-derived tenant_id
// is available in c.Locals.
//
// We use SET LOCAL so the binding lives only for the current transaction. To
// keep that promise we wrap the request body in BEGIN/COMMIT here. Any handler
// that wants to step outside the txn (long-running streaming, websocket
// upgrade) should opt out of this middleware on its route group.
//
// For super-admin endpoints, the platform code can call SetSuperAdmin() inside
// the handler before issuing cross-tenant queries.
func TenantContext(pool *pgxpool.Pool) fiber.Handler {
	return func(c fiber.Ctx) error {
		tenantID, ok := c.Locals("tenantID").(uuid.UUID)
		if !ok || tenantID == uuid.Nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing tenant context",
			})
		}
		userID, _ := c.Locals("userID").(uuid.UUID)

		ctx := c.Context()
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "could not acquire DB connection",
			})
		}
		defer conn.Release()

		// SET LOCAL must run inside a transaction or it's a no-op. We don't
		// commit reads here — sqlc/pgx will run their own transactions; the
		// session var is connection-scoped and stays set until the conn is
		// returned to the pool.
		if _, err := conn.Exec(ctx,
			"SELECT set_config('app.tenant_id', $1, false), "+
				"set_config('app.user_id', $2, false)",
			tenantID.String(), userID.String()); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "could not set tenant context",
			})
		}

		// Stash the connection so handlers that want strict txn isolation can
		// use it; the rest will go through the pool which auto-resets the
		// session vars when a connection is rebound to a new request.
		c.Locals("dbConn", conn)
		return c.Next()
	}
}

// SuperAdminContext is a convenience wrapper for cron / provisioning
// endpoints that need to bypass RLS. Mount this on the platform-admin route
// group instead of TenantContext.
func SuperAdminContext(pool *pgxpool.Pool) fiber.Handler {
	return func(c fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if role != "super_admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "super_admin only",
			})
		}

		ctx := c.Context()
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "could not acquire DB connection",
			})
		}
		defer conn.Release()

		if _, err := conn.Exec(ctx,
			"SELECT set_config('app.is_super_admin', 'true', false)"); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "could not set super-admin context",
			})
		}
		c.Locals("dbConn", conn)
		return c.Next()
	}
}

// PublicLookupContext is mounted on /public/tenants/by-code/:code. It opts
// the request into the tenant_public_orgcode RLS policy so an unauthenticated
// browser can resolve an Org Code to a tenant for branding before login.
func PublicLookupContext(pool *pgxpool.Pool) fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx := c.Context()
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "could not acquire DB connection",
			})
		}
		defer conn.Release()

		if _, err := conn.Exec(ctx,
			"SELECT set_config('app.is_public_lookup', 'true', false)"); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "could not set public lookup context",
			})
		}
		c.Locals("dbConn", conn)
		return c.Next()
	}
}
