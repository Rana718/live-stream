package middleware

import (
	"strings"

	"live-platform/internal/database/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LocalHostTenant is the Locals key CustomDomain sets when the request's
// Host header maps to a tenant's verified domain. Public handlers read it to
// greet a caller by tenant before any JWT is issued. It is NOT the auth
// tenant key — an unauthenticated request still cannot pass TenantContext.
const LocalHostTenant = "hostTenant"

// HostTenant is the minimal tenant info stashed by CustomDomain.
type HostTenant struct {
	ID      uuid.UUID
	OrgCode string
	Name    string
	Slug    string
}

// CustomDomain resolves the request Host to a tenant via tenant_domains
// (opting into the public-lookup RLS policy). No match → c.Next() untouched.
func CustomDomain(pool *pgxpool.Pool) fiber.Handler {
	return func(c fiber.Ctx) error {
		host := strings.ToLower(strings.TrimSpace(c.Hostname()))
		if host == "" {
			return c.Next()
		}
		conn, err := pool.Acquire(c.Context())
		if err != nil {
			return c.Next()
		}
		defer conn.Release()

		if _, err := conn.Exec(c.Context(),
			"SELECT set_config('app.is_public_lookup', 'true', false)"); err != nil {
			return c.Next()
		}
		row, err := db.New(conn).GetTenantByDomain(c.Context(), host)
		if err != nil {
			return c.Next()
		}
		c.Locals(LocalHostTenant, HostTenant{
			ID:      uuid.UUID(row.ID.Bytes),
			OrgCode: row.OrgCode,
			Name:    row.Name,
			Slug:    row.Slug,
		})
		return c.Next()
	}
}

// HostTenantFromCtx reads the resolved host tenant from Fiber Locals.
func HostTenantFromCtx(c fiber.Ctx) (HostTenant, bool) {
	v, ok := c.Locals(LocalHostTenant).(HostTenant)
	return v, ok
}
