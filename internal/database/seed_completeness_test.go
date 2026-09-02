package database_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestFullDemoSeed_EveryTablePopulated asserts that migrations/0133_full_
// platform_seed_demo.sql lands at least one row in every table. It is the
// safety net for that seed doubling as executable documentation of the
// platform flow and as a fixture: a new table shipped without a seed row,
// or a seed INSERT that silently fails a constraint inside the DO block,
// makes this test fail.
//
// Runs against a fully-migrated database (incl. the dev seeds) named by
// SCHEMA_TEST_DATABASE_URL — a superuser connection, so RLS does not hide
// rows. Skipped when unset.
func TestFullDemoSeed_EveryTablePopulated(t *testing.T) {
	conn := schemaTestConn(t)
	ctx := context.Background()

	rows, err := conn.Query(ctx, `
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public'
		  AND tablename <> 'schema_migrations_applied'
		  AND tablename NOT LIKE '%\_default' ESCAPE '\'
		  AND tablename !~ '_[0-9]{6}$'          -- month partitions
		ORDER BY tablename
	`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if len(tables) < 100 {
		t.Fatalf("only %d tables found — DB not fully migrated?", len(tables))
	}

	var empty []string
	for _, tbl := range tables {
		var n int64
		err := conn.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", pgx.Identifier{tbl}.Sanitize())).Scan(&n)
		if err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		if n == 0 {
			empty = append(empty, tbl)
		}
	}

	if len(empty) > 0 {
		sort.Strings(empty)
		t.Fatalf("%d table(s) have no rows after seeding — extend "+
			"migrations/0133_full_platform_seed_demo.sql:\n  %s",
			len(empty), strings.Join(empty, "\n  "))
	}
	t.Logf("all %d tables populated by the demo seeds", len(tables))
}
