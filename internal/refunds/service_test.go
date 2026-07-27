// Integration tests for the refund guard rails: tenant ownership, payment
// status, and provider-payment-id checks. These all run before Issue()
// ever calls out to Razorpay, so they're testable without a live Razorpay
// account or network access — full-refund/partial-refund behavior itself
// (the actual Razorpay API call) isn't covered here.
//
// Skipped automatically if TEST_DATABASE_URL is unset, same convention as
// internal/courseorders/service_test.go.
package refunds_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"live-platform/internal/config"
	"live-platform/internal/database"
	"live-platform/internal/payments"
	"live-platform/internal/refunds"
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

// fixtures creates a tenant + user, and a payment row with the given
// status/provider_payment_id, returning its id.
func fixtures(t *testing.T, pool *pgxpool.Pool, status, providerPaymentID string) (tenantID, paymentID uuid.UUID) {
	t.Helper()
	superCtx := database.WithSuperAdmin(context.Background())

	tenantID = uuid.New()
	if _, err := pool.Exec(superCtx, `
		INSERT INTO tenants (id, org_code, name, slug, plan, status)
		VALUES ($1, $2, $3, $4, 'starter', 'active')
	`, tenantID, "RF"+tenantID.String()[:6], "rf-"+tenantID.String()[:6], "rf-"+tenantID.String()[:6]); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	userID := uuid.New()
	if _, err := pool.Exec(superCtx, `INSERT INTO users (id, tenant_id, role) VALUES ($1, $2, 'student')`, userID, tenantID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	var providerPaymentIDArg any
	if providerPaymentID != "" {
		providerPaymentIDArg = providerPaymentID
	}
	if err := pool.QueryRow(superCtx, `
		INSERT INTO payments (tenant_id, user_id, amount, currency, provider, status, provider_payment_id)
		VALUES ($1, $2, 500, 'INR', 'razorpay', $3, $4)
		RETURNING id
	`, tenantID, userID, status, providerPaymentIDArg).Scan(&paymentID); err != nil {
		t.Fatalf("insert payment: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(superCtx, "DELETE FROM tenants WHERE id = $1", tenantID)
	})
	return tenantID, paymentID
}

func TestIssue_RejectsCrossTenantPayment(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", "secret", "")
	svc := refunds.NewService(pool, rp)

	_, paymentID := fixtures(t, pool, "paid", "pay_abc123")
	otherTenantID := uuid.New() // a tenant that does NOT own this payment

	superCtx := database.WithSuperAdmin(context.Background())
	if _, err := pool.Exec(superCtx, `
		INSERT INTO tenants (id, org_code, name, slug, plan, status)
		VALUES ($1, $2, $3, $4, 'starter', 'active')
	`, otherTenantID, "RFO"+otherTenantID.String()[:6], "rfo-"+otherTenantID.String()[:6], "rfo-"+otherTenantID.String()[:6]); err != nil {
		t.Fatalf("insert other tenant: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(superCtx, "DELETE FROM tenants WHERE id = $1", otherTenantID) })

	ctx := database.WithTenant(context.Background(), otherTenantID.String(), uuid.New().String())
	_, err := svc.Issue(ctx, otherTenantID, refunds.IssueInput{
		PaymentID: paymentID.String(),
		Reason:    "test cross-tenant refund attempt",
	})
	if err == nil {
		t.Fatal("expected refund to be rejected for a payment belonging to a different tenant")
	}
}

func TestIssue_RejectsNonPaidStatus(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", "secret", "")
	svc := refunds.NewService(pool, rp)

	tenantID, paymentID := fixtures(t, pool, "created", "pay_abc123")
	ctx := database.WithTenant(context.Background(), tenantID.String(), uuid.New().String())

	_, err := svc.Issue(ctx, tenantID, refunds.IssueInput{
		PaymentID: paymentID.String(),
		Reason:    "test refund on unpaid order",
	})
	if err == nil {
		t.Fatal("expected refund to be rejected for a payment that was never paid")
	}
}

func TestIssue_RejectsMissingProviderPaymentID(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", "secret", "")
	svc := refunds.NewService(pool, rp)

	tenantID, paymentID := fixtures(t, pool, "paid", "")
	ctx := database.WithTenant(context.Background(), tenantID.String(), uuid.New().String())

	_, err := svc.Issue(ctx, tenantID, refunds.IssueInput{
		PaymentID: paymentID.String(),
		Reason:    "test refund with no razorpay payment id on file",
	})
	if err == nil {
		t.Fatal("expected refund to be rejected when there's no provider_payment_id to refund against")
	}
}

func TestIssue_RejectsInvalidPaymentIDFormat(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", "secret", "")
	svc := refunds.NewService(pool, rp)

	tenantID := uuid.New()
	ctx := database.WithSuperAdmin(context.Background())
	if _, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, org_code, name, slug, plan, status)
		VALUES ($1, $2, $3, $4, 'starter', 'active')
	`, tenantID, "RFI"+tenantID.String()[:6], "rfi-"+tenantID.String()[:6], "rfi-"+tenantID.String()[:6]); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM tenants WHERE id = $1", tenantID) })

	tenantCtx := database.WithTenant(context.Background(), tenantID.String(), uuid.New().String())
	_, err := svc.Issue(tenantCtx, tenantID, refunds.IssueInput{
		PaymentID: "not-a-uuid",
		Reason:    "malformed id",
	})
	if err == nil {
		t.Fatal("expected error for a malformed payment_id")
	}
}
