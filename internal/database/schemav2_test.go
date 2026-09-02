package database_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Schema-v2 invariants that migrations can't self-assert (the RLS + grant
// assertions live in migrations/0110). Runs against a fully-migrated v2
// database named by SCHEMA_TEST_DATABASE_URL (a superuser connection — it
// reads pg_catalog). Skipped when unset.
func schemaTestConn(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("SCHEMA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SCHEMA_TEST_DATABASE_URL not set — skipping schema-v2 assertions")
	}
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// TestNoFloatMoneyColumns: money is bigint minor units. numeric/float/money
// are only allowed for the academic + geo columns in the whitelist.
func TestNoFloatMoneyColumns(t *testing.T) {
	conn := schemaTestConn(t)
	allowed := map[string]bool{
		// academic marks & scores (not money)
		"question_bank.default_marks": true, "question_bank.negative_marks": true,
		"question_bank.numeric_answer": true, "question_bank.numeric_tolerance": true,
		"tests.total_marks": true, "tests.pass_marks": true,
		"test_sections.marks_per_q": true, "test_sections.negative_per_q": true,
		"test_questions.marks": true, "test_questions.negative": true,
		"test_attempts.score": true, "test_attempts.max_score": true,
		"test_responses.numeric_answer": true, "test_responses.marks": true,
		"assignments.max_marks": true, "assignment_submissions.marks_obtained": true,
		// geo
		"attendance.geo_lat": true, "attendance.geo_lng": true,
	}
	rows, err := conn.Query(context.Background(), `
		SELECT table_name, column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND data_type IN ('numeric','double precision','real','money')
		  AND table_name NOT LIKE '%\_2%'
		  AND table_name NOT LIKE '%\_default'
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var bad []string
	for rows.Next() {
		var tbl, col, typ string
		if err := rows.Scan(&tbl, &col, &typ); err != nil {
			t.Fatal(err)
		}
		if !allowed[tbl+"."+col] {
			bad = append(bad, tbl+"."+col+" ("+typ+")")
		}
	}
	if len(bad) > 0 {
		t.Fatalf("non-whitelisted float/numeric columns (money must be bigint minor units):\n  %s",
			strings.Join(bad, "\n  "))
	}
}

// TestEveryForeignKeyIsIndexed: Postgres does not auto-index FK columns;
// an FK column that appears in NO index means a sequential scan of the
// child table every time a referenced row is deleted/updated. Composite
// coverage (FK column present but not leading) is acceptable.
func TestEveryForeignKeyIsIndexed(t *testing.T) {
	conn := schemaTestConn(t)
	rows, err := conn.Query(context.Background(), `
		WITH fk_cols AS (
			SELECT c.conrelid::regclass::text AS tbl,
			       a.attname                  AS col,
			       c.conkey[1]                AS attnum,
			       c.conrelid
			FROM pg_constraint c
			JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
			WHERE c.contype = 'f'
			  AND c.connamespace = 'public'::regnamespace
		)
		SELECT f.tbl, f.col
		FROM fk_cols f
		WHERE NOT EXISTS (
			SELECT 1 FROM pg_index i
			WHERE i.indrelid = f.conrelid
			  AND f.attnum = ANY (i.indkey::int2[])   -- FK col indexed anywhere
		)
		AND f.tbl NOT LIKE '%\_2%'   -- skip dated partitions (parent covers)
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var bad []string
	for rows.Next() {
		var tbl, col string
		if err := rows.Scan(&tbl, &col); err != nil {
			t.Fatal(err)
		}
		bad = append(bad, tbl+"."+col)
	}
	if len(bad) > 0 {
		t.Fatalf("foreign-key columns with no covering index (leading column):\n  %s",
			strings.Join(bad, "\n  "))
	}
}

// TestNoCircularBackPointers: the commerce graph points strictly
// downstream->upstream. These convenience back-pointer columns must not
// exist (see plan "Circular-FK discipline").
func TestNoCircularBackPointers(t *testing.T) {
	conn := schemaTestConn(t)
	forbidden := [][2]string{
		{"orders", "invoice_id"},
		{"orders", "subscription_id"},
		{"orders", "fee_installment_id"},
		{"orders", "payment_id"},
		{"refunds", "credit_note_id"},
		{"payments", "refund_id"},
	}
	for _, f := range forbidden {
		var exists bool
		err := conn.QueryRow(context.Background(), `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema='public' AND table_name=$1 AND column_name=$2
			)`, f[0], f[1]).Scan(&exists)
		if err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Errorf("forbidden back-pointer column exists: %s.%s", f[0], f[1])
		}
	}
}

// TestTenantTablesForceRLS re-checks migrations/0110's assertion from Go so
// a regression fails `go test`, not just a fresh migrate.
func TestTenantTablesForceRLS(t *testing.T) {
	conn := schemaTestConn(t)
	rows, err := conn.Query(context.Background(), `
		SELECT DISTINCT c.table_name
		FROM information_schema.columns c
		JOIN pg_class pc ON pc.relname = c.table_name
		JOIN pg_namespace n ON n.oid = pc.relnamespace AND n.nspname='public'
		WHERE c.table_schema='public' AND c.column_name='tenant_id'
		  AND c.is_nullable='NO' AND pc.relkind IN ('r','p')
		  AND c.table_name <> 'refresh_tokens'
		  AND c.table_name NOT LIKE '%\_2%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var tn string
		if err := rows.Scan(&tn); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, tn)
	}
	for _, tn := range tables {
		var forced bool
		if err := conn.QueryRow(context.Background(),
			`SELECT relrowsecurity AND relforcerowsecurity FROM pg_class
			 WHERE relname=$1 AND relnamespace='public'::regnamespace`, tn).Scan(&forced); err != nil {
			t.Fatal(err)
		}
		if !forced {
			t.Errorf("%s: RLS not ENABLE+FORCE", tn)
		}
		for _, pol := range []string{"tenant_isolation_" + tn, "super_admin_" + tn} {
			var has bool
			if err := conn.QueryRow(context.Background(),
				`SELECT EXISTS(SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename=$1 AND policyname=$2)`,
				tn, pol).Scan(&has); err != nil {
				t.Fatal(err)
			}
			if !has {
				t.Errorf("%s: missing policy %s", tn, pol)
			}
		}
	}
}
