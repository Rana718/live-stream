// Integration tests for the money-critical half of course purchase:
// signature verification, cross-user tampering, and idempotent
// re-verification. Buy() isn't covered here — it calls out to Razorpay's
// CreateOrder API over the network, which these tests deliberately don't
// depend on (see internal/payments for the pure-function signature tests
// that don't need a live Razorpay account either).
//
// Skipped automatically if TEST_DATABASE_URL is unset, same convention as
// internal/middleware/tenant_isolation_test.go and
// internal/database/postgres_test.go.
package courseorders_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"live-platform/internal/config"
	"live-platform/internal/courseorders"
	"live-platform/internal/database"
	"live-platform/internal/payments"
)

const testKeySecret = "test_key_secret"

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

// fixtures creates one tenant, one course (price 499.00), and one pending
// course-order (payments row, status=created) for a given buyer. Returns
// the order's Razorpay order ID and a cleanup func.
func fixtures(t *testing.T, pool *pgxpool.Pool, buyerID uuid.UUID) (tenantID, courseID uuid.UUID, orderID string) {
	t.Helper()
	superCtx := database.WithSuperAdmin(context.Background())

	tenantID = uuid.New()
	if _, err := pool.Exec(superCtx, `
		INSERT INTO tenants (id, org_code, name, slug, plan, status)
		VALUES ($1, $2, $3, $4, 'starter', 'active')
	`, tenantID, "CO"+tenantID.String()[:6], "co-"+tenantID.String()[:6], "co-"+tenantID.String()[:6]); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	if _, err := pool.Exec(superCtx, `
		INSERT INTO users (id, tenant_id, role) VALUES ($1, $2, 'student')
	`, buyerID, tenantID); err != nil {
		t.Fatalf("insert buyer: %v", err)
	}

	courseID = uuid.New()
	if _, err := pool.Exec(superCtx, `
		INSERT INTO courses (id, tenant_id, title, slug, price)
		VALUES ($1, $2, 'Test Course', $3, 499.00)
	`, courseID, tenantID, "co-slug-"+courseID.String()[:8]); err != nil {
		t.Fatalf("insert course: %v", err)
	}

	orderID = "order_" + uuid.New().String()[:12]
	if _, err := pool.Exec(superCtx, `
		INSERT INTO payments (tenant_id, user_id, course_id, amount, currency, provider, provider_order_id, status, metadata)
		VALUES ($1, $2, $3, 499.00, 'INR', 'razorpay', $4, 'created', '{}')
	`, tenantID, buyerID, courseID, orderID); err != nil {
		t.Fatalf("insert payment: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(superCtx, "DELETE FROM tenants WHERE id = $1", tenantID)
	})
	return tenantID, courseID, orderID
}

func TestVerify_RejectsInvalidSignature(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", testKeySecret, "")
	svc := courseorders.NewService(pool, rp)

	buyerID := uuid.New()
	tenantID, _, orderID := fixtures(t, pool, buyerID)
	ctx := database.WithTenant(context.Background(), tenantID.String(), buyerID.String())

	_, err := svc.Verify(ctx, courseorders.VerifyRequest{
		RazorpayOrderID:   orderID,
		RazorpayPaymentID: "pay_x",
		RazorpaySignature: "not-a-real-signature",
	}, buyerID)
	if err == nil {
		t.Fatal("expected signature mismatch error")
	}
}

func TestVerify_RejectsWrongUser(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", testKeySecret, "")
	svc := courseorders.NewService(pool, rp)

	buyerID := uuid.New()
	attackerID := uuid.New()
	tenantID, _, orderID := fixtures(t, pool, buyerID)

	sig := computeSignature(t, orderID, "pay_x", testKeySecret)
	// The attacker knows a valid-looking signature (e.g. leaked from a
	// client-side log) but isn't the user who owns this order.
	ctx := database.WithTenant(context.Background(), tenantID.String(), attackerID.String())
	_, err := svc.Verify(ctx, courseorders.VerifyRequest{
		RazorpayOrderID:   orderID,
		RazorpayPaymentID: "pay_x",
		RazorpaySignature: sig,
	}, attackerID)
	if err == nil {
		t.Fatal("expected 'not your order' error for a different user")
	}
}

func TestVerify_SuccessAndIdempotency(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", testKeySecret, "")
	svc := courseorders.NewService(pool, rp)

	buyerID := uuid.New()
	tenantID, courseID, orderID := fixtures(t, pool, buyerID)
	sig := computeSignature(t, orderID, "pay_x", testKeySecret)
	ctx := database.WithTenant(context.Background(), tenantID.String(), buyerID.String())
	req := courseorders.VerifyRequest{
		RazorpayOrderID:   orderID,
		RazorpayPaymentID: "pay_x",
		RazorpaySignature: sig,
	}

	payment, err := svc.Verify(ctx, req, buyerID)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if payment.Status.String != "paid" {
		t.Fatalf("expected status=paid, got %q", payment.Status.String)
	}

	var enrollCount int
	superCtx := database.WithSuperAdmin(context.Background())
	if err := pool.QueryRow(superCtx,
		"SELECT count(*) FROM enrollments WHERE user_id = $1 AND course_id = $2",
		buyerID, courseID).Scan(&enrollCount); err != nil {
		t.Fatalf("count enrollments: %v", err)
	}
	if enrollCount != 1 {
		t.Fatalf("expected exactly 1 enrollment after verify, got %d", enrollCount)
	}

	// Re-verify: must not error, and must not create a second enrollment
	// (the button-spam / webhook-plus-client-verify-both-fire case).
	payment2, err := svc.Verify(ctx, req, buyerID)
	if err != nil {
		t.Fatalf("idempotent re-verify: %v", err)
	}
	if payment2.Status.String != "paid" {
		t.Fatalf("expected status=paid on re-verify, got %q", payment2.Status.String)
	}
	if err := pool.QueryRow(superCtx,
		"SELECT count(*) FROM enrollments WHERE user_id = $1 AND course_id = $2",
		buyerID, courseID).Scan(&enrollCount); err != nil {
		t.Fatalf("count enrollments after re-verify: %v", err)
	}
	if enrollCount != 1 {
		t.Fatalf("expected still exactly 1 enrollment after idempotent re-verify, got %d", enrollCount)
	}
}

func TestVerify_UnknownOrder(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", testKeySecret, "")
	svc := courseorders.NewService(pool, rp)

	buyerID := uuid.New()
	tenantID, _, _ := fixtures(t, pool, buyerID)
	sig := computeSignature(t, "order_does_not_exist", "pay_x", testKeySecret)
	ctx := database.WithTenant(context.Background(), tenantID.String(), buyerID.String())

	_, err := svc.Verify(ctx, courseorders.VerifyRequest{
		RazorpayOrderID:   "order_does_not_exist",
		RazorpayPaymentID: "pay_x",
		RazorpaySignature: sig,
	}, buyerID)
	if err == nil {
		t.Fatal("expected 'order not found' error")
	}
}

// computeSignature mirrors payments.Razorpay.VerifyPaymentSignature's
// algorithm exactly (HMAC-SHA256 of "orderID|paymentID", hex-encoded) —
// there's no exported "sign" helper, VerifyPaymentSignature is the only
// exported surface, so tests compute what a real Razorpay checkout
// response would have sent.
func computeSignature(t *testing.T, orderID, paymentID, keySecret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(keySecret))
	mac.Write([]byte(orderID + "|" + paymentID))
	return hex.EncodeToString(mac.Sum(nil))
}
