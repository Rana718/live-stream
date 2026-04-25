// Cross-tenant RLS smoke test. docs.md §20 calls this out as the highest-
// risk surface ("tenant data leak via RLS misconfig — automated tests for
// cross-tenant queries"). The test:
//   1. Creates two tenants + one user inside each.
//   2. Inserts a course in each tenant.
//   3. With app.tenant_id set to tenant A, confirms only tenant A's
//      course is visible — even via a SELECT that doesn't mention
//      tenant_id.
//   4. Confirms an UPDATE that tries to bump tenant B's course while
//      app.tenant_id is set to A returns 0 rows.
//   5. Confirms super_admin bypass returns both rows.
//
// Skipped automatically if TEST_DATABASE_URL is unset — keeps the unit
// test layer fast and lets CI opt in.
package middleware_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func skipIfNoDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping RLS integration test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return pool
}

// withTenant returns a connection that has app.tenant_id session-set.
// Caller is responsible for releasing it.
func withTenant(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID) *pgxpool.Conn {
	t.Helper()
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := conn.Exec(context.Background(),
		"SELECT set_config('app.tenant_id', $1, false)",
		tenant.String()); err != nil {
		conn.Release()
		t.Fatalf("set_config: %v", err)
	}
	return conn
}

func TestRLS_TenantIsolation(t *testing.T) {
	pool := skipIfNoDB(t)
	defer pool.Close()
	ctx := context.Background()

	// Bootstrap as super_admin so we can freely insert into both tenants.
	root, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer root.Release()
	if _, err := root.Exec(ctx,
		"SELECT set_config('app.is_super_admin', 'true', false)"); err != nil {
		t.Fatalf("super_admin set: %v", err)
	}

	// Create two tenants.
	tenantA := uuid.New()
	tenantB := uuid.New()
	for _, id := range []uuid.UUID{tenantA, tenantB} {
		if _, err := root.Exec(ctx, `
			INSERT INTO tenants (id, org_code, name, slug, plan, status)
			VALUES ($1, $2, $3, $4, 'starter', 'active')
		`, id, "TEST"+id.String()[:6], "test-"+id.String()[:6], "test-"+id.String()[:6]); err != nil {
			t.Fatalf("insert tenant: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = root.Exec(ctx, "DELETE FROM tenants WHERE id IN ($1, $2)", tenantA, tenantB)
	})

	// One course per tenant. tenant_id is required so RLS-bypass via
	// super_admin is the only way to populate both.
	courseA := uuid.New()
	courseB := uuid.New()
	for _, x := range []struct {
		course uuid.UUID
		tenant uuid.UUID
		title  string
	}{{courseA, tenantA, "TENANT_A_COURSE"}, {courseB, tenantB, "TENANT_B_COURSE"}} {
		if _, err := root.Exec(ctx, `
			INSERT INTO courses (id, tenant_id, title)
			VALUES ($1, $2, $3)
		`, x.course, x.tenant, x.title); err != nil {
			t.Fatalf("insert course: %v", err)
		}
	}

	// 1. Tenant A only sees its own course.
	connA := withTenant(t, pool, tenantA)
	defer connA.Release()
	var seen string
	row := connA.QueryRow(ctx, "SELECT title FROM courses WHERE id = $1", courseA)
	if err := row.Scan(&seen); err != nil {
		t.Fatalf("tenant A reading own course: %v", err)
	}
	if seen != "TENANT_A_COURSE" {
		t.Fatalf("expected TENANT_A_COURSE, got %q", seen)
	}

	// 2. Tenant A *cannot* see tenant B's course — RLS hides it as if it
	//    didn't exist (no row found rather than permission error).
	row = connA.QueryRow(ctx, "SELECT title FROM courses WHERE id = $1", courseB)
	if err := row.Scan(&seen); err == nil {
		t.Fatalf("RLS LEAK: tenant A read tenant B course (got %q)", seen)
	}

	// 3. Tenant A's UPDATE on tenant B's row affects 0 rows.
	tag, err := connA.Exec(ctx,
		"UPDATE courses SET title = 'HIJACKED' WHERE id = $1", courseB)
	if err != nil {
		t.Fatalf("update across tenant: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Fatalf("RLS LEAK: tenant A updated %d rows in tenant B", tag.RowsAffected())
	}

	// 4. super_admin sees both rows.
	var count int
	if err := root.QueryRow(ctx,
		"SELECT count(*) FROM courses WHERE id IN ($1, $2)",
		courseA, courseB).Scan(&count); err != nil {
		t.Fatalf("super_admin count: %v", err)
	}
	if count != 2 {
		t.Fatalf("super_admin expected 2 rows, got %d", count)
	}
}
