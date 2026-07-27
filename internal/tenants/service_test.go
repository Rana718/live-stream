// Regression test for Service.DomainIsRegistered — the guard behind
// Caddy's on-demand TLS "ask" endpoint (see CaddyAskDomain in handler.go
// and docker/Caddyfile). A false positive here would let Caddy attempt
// Let's Encrypt issuance for domains that aren't actually ours.
//
// Skipped automatically if TEST_DATABASE_URL is unset, same convention as
// internal/courseorders/service_test.go.
package tenants_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"live-platform/internal/config"
	"live-platform/internal/database"
	"live-platform/internal/tenants"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping")
	}
	pool, err := database.NewPostgresPool(&config.DatabaseConfig{
		Host: "localhost", Port: "5432",
		User: "app_user", Password: "app_user_dev_password",
		DBName: "live_platform", SSLMode: "disable",
	})
	if err != nil {
		t.Fatalf("connect as app_user: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestDomainIsRegistered(t *testing.T) {
	pool := testPool(t)
	svc := tenants.NewService(pool)
	superCtx := database.WithSuperAdmin(context.Background())

	tenantID := uuid.New()
	domain := "caddy-ask-test-" + tenantID.String()[:8] + ".example.com"
	if _, err := pool.Exec(superCtx, `
		INSERT INTO tenants (id, org_code, name, slug, plan, status, custom_domain)
		VALUES ($1, $2, $3, $4, 'starter', 'active', $5)
	`, tenantID, "CA"+tenantID.String()[:6], "ca-"+tenantID.String()[:6], "ca-"+tenantID.String()[:6], domain); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(superCtx, "DELETE FROM tenants WHERE id = $1", tenantID)
	})

	// The ask endpoint runs unauthenticated (Caddy calling before any
	// tenant/user is known), so use a public-lookup context here too.
	pubCtx := database.WithPublicLookup(context.Background())

	if !svc.DomainIsRegistered(pubCtx, domain) {
		t.Fatalf("expected %q to be recognized as registered", domain)
	}
	if svc.DomainIsRegistered(pubCtx, "definitely-not-ours.example.com") {
		t.Fatal("expected an unregistered domain to return false")
	}
}
