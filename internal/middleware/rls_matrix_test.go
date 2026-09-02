package middleware_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// noRLSByDesign — internal queue / idempotency tables that deliberately carry
// no RLS (no per-tenant reads happen through them; workers run cross-tenant).
var noRLSByDesign = map[string]bool{
	"jobs": true, "outbox": true, "webhook_events": true,
}

// TestRLS_EveryTenantTableIsForced is a static guard: every base table or
// partition that carries a tenant_id column (minus the internal-queue
// allowlist) must have RLS ENABLEd + FORCEd and at least one policy.
// Migration 0110 asserts a stricter version at migrate time; this catches a
// new table shipped without apply_tenant_rls() in CI, and also covers
// partitions (0110 predates some of them).
func TestRLS_EveryTenantTableIsForced(t *testing.T) {
	pool := skipIfNoDB(t)
	defer pool.Close()
	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity,
		       (SELECT count(*) FROM pg_policy p WHERE p.polrelid = c.oid) AS npol
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		JOIN information_schema.columns col
		     ON col.table_schema = 'public' AND col.table_name = c.relname
		    AND col.column_name = 'tenant_id'
		WHERE c.relkind IN ('r', 'p')
		ORDER BY c.relname
	`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var checked int
	var problems []string
	for rows.Next() {
		var name string
		var enabled, forced bool
		var npol int
		if err := rows.Scan(&name, &enabled, &forced, &npol); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if noRLSByDesign[name] {
			continue
		}
		checked++
		var miss []string
		if !enabled {
			miss = append(miss, "not ENABLEd")
		}
		if !forced {
			miss = append(miss, "not FORCEd")
		}
		if npol == 0 {
			miss = append(miss, "no policies")
		}
		if len(miss) > 0 {
			problems = append(problems, fmt.Sprintf("  %s: %s", name, strings.Join(miss, ", ")))
		}
	}
	if checked < 50 {
		t.Fatalf("only %d tenant tables checked — schema not fully migrated?", checked)
	}
	if len(problems) > 0 {
		t.Fatalf("%d tenant table(s)/partition(s) with RLS gaps:\n%s", len(problems), strings.Join(problems, "\n"))
	}
	t.Logf("%d tenant-scoped tables/partitions — all ENABLEd + FORCEd with policies", checked)
}

// TestRLS_DenyMatrix populates a row in tenant B for a set of core tables,
// then confirms tenant A can neither SELECT, UPDATE nor DELETE it.
func TestRLS_DenyMatrix(t *testing.T) {
	pool := skipIfNoDB(t)
	defer pool.Close()
	ctx := context.Background()

	root, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer root.Release()
	if _, err := root.Exec(ctx, "SELECT set_config('app.is_super_admin','true',false)"); err != nil {
		t.Fatalf("super_admin: %v", err)
	}

	tA, tB := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{tA, tB} {
		if _, err := root.Exec(ctx,
			`INSERT INTO tenants (id, org_code, name, slug, plan, status, place_of_supply)
			 VALUES ($1,$2,$3,$4,'starter','active','09')`,
			id, "MTX"+id.String()[:6], "mtx-"+id.String()[:6], "mtx-"+id.String()[:6]); err != nil {
			t.Fatalf("seed tenant: %v", err)
		}
	}
	defer func() { _, _ = root.Exec(ctx, "DELETE FROM tenants WHERE id IN ($1,$2)", tA, tB) }()

	seedUser := uuid.New()
	if _, err := root.Exec(ctx, `INSERT INTO users (id, full_name, status) VALUES ($1,'Mtx User','active')`, seedUser); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	defer func() { _, _ = root.Exec(ctx, "DELETE FROM users WHERE id=$1", seedUser) }()

	// Seed a course in tenant B first — batches / lessons reference it.
	courseB := uuid.New()
	if _, err := root.Exec(ctx,
		`INSERT INTO courses (id, tenant_id, title, slug) VALUES ($1,$2,'MTX Course',$3)`,
		courseB, tB, "mtx-"+courseB.String()[:8]); err != nil {
		t.Fatalf("seed course: %v", err)
	}
	defer func() { _, _ = root.Exec(ctx, "DELETE FROM courses WHERE id=$1", courseB) }()

	cases := []struct {
		table string
		// insert takes ($1 rowID, $2 tenantID) and any extras appended.
		insert string
		extra  []any
	}{
		{"subjects", `INSERT INTO subjects (id, tenant_id, name) VALUES ($1,$2,'MTX Subject')`, nil},
		{"batches", `INSERT INTO batches (id, tenant_id, course_id, name) VALUES ($1,$2,$3,'MTX Batch')`, []any{courseB}},
		{"tenant_users", `INSERT INTO tenant_users (id, tenant_id, user_id, role, status) VALUES ($1,$2,$3,'student','active')`, []any{seedUser}},
		{"enrollments", `INSERT INTO enrollments (id, tenant_id, user_id, course_id) VALUES ($1,$2,$3,$4)`, []any{seedUser, courseB}},
		{"notifications", `INSERT INTO notifications (id, tenant_id, user_id, template_key, title, body) VALUES ($1,$2,$3,'mtx','MTX','hi')`, []any{seedUser}},
		{"orders", `INSERT INTO orders (id, tenant_id, user_id, code, subtotal_minor, total_minor) VALUES ($1,$2,$3,'MTX-1',0,0)`, []any{seedUser}},
		{"audit_logs", `INSERT INTO audit_logs (id, tenant_id, actor_user_id, action, entity_type) VALUES ($1,$2,$3,'mtx.test','course')`, []any{seedUser}},
	}

	for _, cse := range cases {
		t.Run(cse.table, func(t *testing.T) {
			rowID := uuid.New()
			args := append([]any{rowID, tB}, cse.extra...)
			if _, err := root.Exec(ctx, cse.insert, args...); err != nil {
				t.Fatalf("seed %s: %v", cse.table, err)
			}
			defer func() {
				_, _ = root.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE id=$1", cse.table), rowID)
			}()

			connA := withTenant(t, pool, tA)
			defer connA.Release()

			var n int
			if err := connA.QueryRow(ctx,
				fmt.Sprintf("SELECT count(*) FROM %s WHERE id=$1", cse.table), rowID,
			).Scan(&n); err != nil && err != pgx.ErrNoRows {
				t.Fatalf("select: %v", err)
			}
			if n != 0 {
				t.Fatalf("RLS LEAK: tenant A SELECT saw tenant B's %s row", cse.table)
			}

			tag, err := connA.Exec(ctx,
				fmt.Sprintf("UPDATE %s SET tenant_id = tenant_id WHERE id=$1", cse.table), rowID)
			if err != nil {
				t.Fatalf("update: %v", err)
			}
			if tag.RowsAffected() != 0 {
				t.Fatalf("RLS LEAK: tenant A UPDATEd %d of tenant B's %s", tag.RowsAffected(), cse.table)
			}

			tag, err = connA.Exec(ctx,
				fmt.Sprintf("DELETE FROM %s WHERE id=$1", cse.table), rowID)
			if err != nil {
				t.Fatalf("delete: %v", err)
			}
			if tag.RowsAffected() != 0 {
				t.Fatalf("RLS LEAK: tenant A DELETEd %d of tenant B's %s", tag.RowsAffected(), cse.table)
			}
		})
	}
}
