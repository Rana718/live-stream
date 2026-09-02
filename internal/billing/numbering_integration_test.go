package billing_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

// Gapless-numbering is the one real concurrency hazard in billing: N paid
// orders settling at once must produce contiguous invoice sequence numbers
// with no gaps and no dupes. AllocateInvoiceNumber row-locks the series row
// so concurrent allocations serialise; this proves it end to end.
//
// Skipped when TEST_DATABASE_URL is unset (an app_user connection — the same
// convention as the RLS isolation test).
func TestInvoiceNumberingIsGaplessUnderConcurrency(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping billing numbering integration test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	tenantID := uuid.New()

	// Seed + ensure-series in one super-admin transaction (this pool has no
	// BeforeAcquire hook, so we set the GUC explicitly).
	seed, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin seed: %v", err)
	}
	if _, err := seed.Exec(ctx, "SET LOCAL app.is_super_admin = 'true'"); err != nil {
		t.Fatalf("set super_admin: %v", err)
	}
	if _, err := seed.Exec(ctx,
		`INSERT INTO tenants (id, org_code, name, slug, status, plan, place_of_supply)
		 VALUES ($1, $2, 'Billing Test', $2, 'active', 'starter', '09')`,
		tenantID, "BILL"+tenantID.String()[:8]); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	const fy = "2099-00"
	if err := db.New(pool).WithTx(seed).EnsureInvoiceSeries(ctx, db.EnsureInvoiceSeriesParams{
		TenantID: pgUUID(tenantID), FinYear: fy,
	}); err != nil {
		t.Fatalf("ensure series: %v", err)
	}
	if err := seed.Commit(ctx); err != nil {
		t.Fatalf("commit seed: %v", err)
	}
	t.Cleanup(func() {
		c, err := pool.Begin(context.Background())
		if err != nil {
			return
		}
		_, _ = c.Exec(context.Background(), "SET LOCAL app.is_super_admin = 'true'")
		_, _ = c.Exec(context.Background(), "DELETE FROM tenants WHERE id=$1", tenantID)
		_ = c.Commit(context.Background())
	})

	const n = 50
	seqs := make([]int64, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Errorf("begin: %v", err)
				return
			}
			defer func() { _ = tx.Commit(ctx) }()
			if _, err := tx.Exec(ctx, "SET LOCAL app.is_super_admin = 'true'"); err != nil {
				t.Errorf("set guc: %v", err)
				return
			}
			row, err := db.New(pool).WithTx(tx).AllocateInvoiceNumber(ctx, db.AllocateInvoiceNumberParams{
				TenantID: pgUUID(tenantID), FinYear: fy,
			})
			if err != nil {
				t.Errorf("allocate: %v", err)
				return
			}
			seqs[i] = row.Seq
		}(i)
	}
	wg.Wait()

	seen := map[int64]bool{}
	var min, max int64 = 1<<62, 0
	for _, s := range seqs {
		if seen[s] {
			t.Fatalf("duplicate sequence %d", s)
		}
		seen[s] = true
		if s < min {
			min = s
		}
		if s > max {
			max = s
		}
	}
	if max-min != n-1 {
		t.Fatalf("range [%d,%d] is not %d contiguous values", min, max, n)
	}
}
