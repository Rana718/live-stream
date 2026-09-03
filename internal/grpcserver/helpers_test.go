package grpcserver_test

import (
	"context"
	"os"
	"testing"

	"live-platform/internal/config"
	"live-platform/internal/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping")
	}
	dbName := os.Getenv("TEST_DB_NAME")
	if dbName == "" {
		dbName = "live_platform"
	}
	pool, err := database.NewPostgresPool(&config.DatabaseConfig{
		Host: "localhost", Port: "5432",
		User: "app_user", Password: "app_user_dev_password",
		DBName: dbName, SSLMode: "disable",
	})
	if err != nil {
		t.Fatalf("connect as app_user: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seedTenantAndOwner inserts a throwaway tenant + owner membership and returns
// their ids. Cleaned up on test end.
func seedTenantAndOwner(t *testing.T, pool *pgxpool.Pool) (tenantID, userID uuid.UUID) {
	t.Helper()
	ctx := database.WithSuperAdmin(context.Background())
	tenantID, userID = uuid.New(), uuid.New()
	sfx := tenantID.String()[:8]
	if _, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, org_code, name, slug, plan, status)
		VALUES ($1, $2, $3, $4, 'starter', 'active')`,
		tenantID, "GT"+sfx[:6], "gt-"+sfx, "gt-"+sfx); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, full_name, status) VALUES ($1, $2, 'active')`,
		userID, "GRPC Test Owner"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tenant_users (tenant_id, user_id, role, status)
		VALUES ($1, $2, 'owner', 'active')`, tenantID, userID); err != nil {
		t.Fatalf("insert membership: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM tenant_users WHERE tenant_id=$1", tenantID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id=$1", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM tenants WHERE id=$1", tenantID)
	})
	return tenantID, userID
}
