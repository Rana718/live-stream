// Regression test for the superuser/RLS-bypass finding documented in
// migrations/043_restricted_app_role.sql: connecting as `postgres` (a
// Postgres superuser) silently disabled every row-level-security policy
// in every environment this app ever ran in. NewPostgresPool must refuse
// to start against a superuser/BYPASSRLS role, and must succeed against
// the restricted app_user role.
//
// Skipped automatically if TEST_DATABASE_URL is unset, same convention as
// internal/middleware/tenant_isolation_test.go.
package database_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"live-platform/internal/config"
	"live-platform/internal/database"
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

// TestBeforeAcquire_ScopesQueriesByContext is the real end-to-end proof for
// the fix in migrations/043_restricted_app_role.sql +
// internal/middleware/tenant.go: a plain database.WithTenant(ctx, ...)
// context, used with the *shared pool* exactly the way every service in
// this codebase does (db.New(pgPool), not a pinned connection), must
// correctly scope queries via RLS — since BeforeAcquire is what actually
// applies the session GUCs now, not the middleware directly.
func TestBeforeAcquire_ScopesQueriesByContext(t *testing.T) {
	pool := testPool(t)
	bg := context.Background()

	tenantA := uuid.New()
	tenantB := uuid.New()
	superCtx := database.WithSuperAdmin(bg)
	for _, id := range []uuid.UUID{tenantA, tenantB} {
		if _, err := pool.Exec(superCtx, `
			INSERT INTO tenants (id, org_code, name, slug, plan, status)
			VALUES ($1, $2, $3, $4, 'starter', 'active')
		`, id, "BA"+id.String()[:6], "ba-"+id.String()[:6], "ba-"+id.String()[:6]); err != nil {
			t.Fatalf("insert tenant: %v", err)
		}
	}
	defer func() {
		_, _ = pool.Exec(superCtx, "DELETE FROM tenants WHERE id IN ($1, $2)", tenantA, tenantB)
	}()

	courseA := uuid.New()
	courseB := uuid.New()
	for _, x := range []struct {
		course, tenant uuid.UUID
		title          string
	}{{courseA, tenantA, "BA_COURSE_A"}, {courseB, tenantB, "BA_COURSE_B"}} {
		if _, err := pool.Exec(superCtx, `
			INSERT INTO courses (id, tenant_id, title, slug)
			VALUES ($1, $2, $3, $4)
		`, x.course, x.tenant, x.title, "ba-slug-"+x.course.String()[:8]); err != nil {
			t.Fatalf("insert course: %v", err)
		}
	}

	// The actual pattern every handler/service uses: a plain context.Background()
	// tagged via database.WithTenant, run against the shared pool.
	tenantACtx := database.WithTenant(bg, tenantA.String(), uuid.New().String())

	var seen string
	err := pool.QueryRow(tenantACtx, "SELECT title FROM courses WHERE id = $1", courseA).Scan(&seen)
	if err != nil {
		t.Fatalf("tenant A reading own course via shared pool: %v", err)
	}
	if seen != "BA_COURSE_A" {
		t.Fatalf("expected BA_COURSE_A, got %q", seen)
	}

	err = pool.QueryRow(tenantACtx, "SELECT title FROM courses WHERE id = $1", courseB).Scan(&seen)
	if err == nil {
		t.Fatalf("RLS LEAK via shared pool: tenant A context read tenant B's course (got %q)", seen)
	}

	// A plain context.Background() with *no* RLS context attached must see
	// nothing — the safe default is closed, not "whatever the last query
	// on this pooled connection happened to leave set."
	err = pool.QueryRow(bg, "SELECT title FROM courses WHERE id = $1", courseA).Scan(&seen)
	if err == nil {
		t.Fatalf("expected a bare context.Background() query to see nothing, got %q", seen)
	}

	// super_admin context sees both, still via the shared pool.
	var count int
	if err := pool.QueryRow(superCtx,
		"SELECT count(*) FROM courses WHERE id IN ($1, $2)", courseA, courseB).Scan(&count); err != nil {
		t.Fatalf("super_admin count: %v", err)
	}
	if count != 2 {
		t.Fatalf("super_admin expected 2 rows, got %d", count)
	}
}

func TestNewPostgresPool_RefusesSuperuser(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping")
	}
	cfg := &config.DatabaseConfig{
		Host: "localhost", Port: "5432",
		User: "postgres", Password: "postgres",
		DBName: "live_platform", SSLMode: "disable",
	}
	_, err := database.NewPostgresPool(cfg)
	if err == nil {
		t.Fatal("expected NewPostgresPool to refuse a superuser DB_USER (RLS bypass), got nil error")
	}
}

func TestNewPostgresPool_AcceptsRestrictedRole(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping")
	}
	cfg := &config.DatabaseConfig{
		Host: "localhost", Port: "5432",
		User: "app_user", Password: "app_user_dev_password",
		DBName: "live_platform", SSLMode: "disable",
	}
	pool, err := database.NewPostgresPool(cfg)
	if err != nil {
		t.Fatalf("expected the restricted app_user role to connect cleanly, got: %v", err)
	}
	pool.Close()
}
