package middleware

import (
	"context"
	"strings"

	"live-platform/internal/database/db"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CustomDomain attaches the implied tenant to a request when the Host
// header matches a tenant's `custom_domain`. Used at the edge of the public
// surfaces (`/public/...`, marketing pages) so clients on a Premium tier
// don't have to type an Org Code on their own subdomain.
//
// We look up via a separate connection so we can opt the query into the
// `tenant_public_orgcode` RLS policy (which already permits SELECT-by-host
// without an authenticated session).
//
// If the host doesn't match any tenant row, the middleware just calls
// c.Next() — non-tenant routes (super_admin, /health, /metrics) work as
// before.
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

		// Opt into the public-lookup policy so the SELECT works without a
		// tenant_id session var. We can't reuse c.Locals("dbConn") here —
		// CustomDomain runs ahead of TenantContext in the chain.
		if _, err := conn.Exec(c.Context(),
			"SELECT set_config('app.is_public_lookup', 'true', false)"); err != nil {
			return c.Next()
		}

		row, err := db.New(conn).GetTenantByDomain(c.Context(), pgtype.Text{
			String: host,
			Valid:  true,
		})
		if err != nil {
			return c.Next()
		}

		// Stash the resolved tenant for downstream handlers. We don't set
		// c.Locals("tenantID") with the JWT key — that would let an
		// unauthenticated request pass tenant-context middleware. Instead
		// the org-code public-lookup endpoint (and similar public surfaces)
		// can read this hint via c.Locals("hostTenant").
		c.Locals("hostTenant", db.Tenant{
			ID:       row.ID,
			OrgCode:  row.OrgCode,
			Name:     row.Name,
			Slug:     row.Slug,
			Plan:     row.Plan,
			Status:   row.Status,
		})
		return c.Next()
	}
}

// HostTenantFromCtx is a small helper for handlers that want to greet the
// caller by tenant before they've issued a JWT (eg the marketing landing
// served on a tenant's custom domain).
func HostTenantFromCtx(ctx context.Context) (uuid string, ok bool) {
	v := ctx.Value("hostTenant")
	if v == nil {
		return "", false
	}
	if t, k := v.(db.Tenant); k {
		// We don't use uuid.UUID here to avoid pulling the package into a
		// hot middleware path. Caller can re-parse if needed.
		return string(t.ID.Bytes[:]), true
	}
	return "", false
}
