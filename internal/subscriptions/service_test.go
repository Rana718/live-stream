// Integration tests for the subscription checkout/verify/cancel flow.
// Skipped automatically if TEST_DATABASE_URL is unset, same convention as
// internal/courseorders/service_test.go.
package subscriptions_test

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
	"live-platform/internal/database"
	"live-platform/internal/payments"
	"live-platform/internal/subscriptions"
)

const testKeySecret = "sub_test_key_secret"

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

// fixtures creates a tenant + a user. Free/paid plans are created per-test
// since price varies by test case.
func fixtures(t *testing.T, pool *pgxpool.Pool) (tenantID, userID uuid.UUID) {
	t.Helper()
	superCtx := database.WithSuperAdmin(context.Background())

	tenantID = uuid.New()
	if _, err := pool.Exec(superCtx, `
		INSERT INTO tenants (id, org_code, name, slug, plan, status)
		VALUES ($1, $2, $3, $4, 'starter', 'active')
	`, tenantID, "SB"+tenantID.String()[:6], "sb-"+tenantID.String()[:6], "sb-"+tenantID.String()[:6]); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	userID = uuid.New()
	if _, err := pool.Exec(superCtx, `INSERT INTO users (id, tenant_id, role) VALUES ($1, $2, 'student')`, userID, tenantID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(superCtx, "DELETE FROM tenants WHERE id = $1", tenantID)
	})
	return tenantID, userID
}

func createPlan(t *testing.T, ctx context.Context, svc *subscriptions.Service, tenantID uuid.UUID, price float64) uuid.UUID {
	t.Helper()
	p, err := svc.CreatePlan(ctx, tenantID, subscriptions.UpsertPlanRequest{
		Name:         "Plan-" + uuid.New().String()[:8],
		Slug:         "plan-" + uuid.New().String()[:8],
		Price:        price,
		DurationDays: 30,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return uuid.UUID(p.ID.Bytes)
}

func TestStartCheckout_FreePlanActivatesImmediately(t *testing.T) {
	pool := testPool(t)
	svc := subscriptions.NewService(pool, nil)
	tenantID, userID := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	// CreatePlan itself needs a tenant-scoped ctx to satisfy RLS on
	// subscription_plans, but the tenantID param is what actually gets
	// written — pass the real one, not uuid.Nil, for a real plan.
	p, err := svc.CreatePlan(ctx, tenantID, subscriptions.UpsertPlanRequest{
		Name: "Free Plan", Slug: "free-plan-" + userID.String()[:8],
		Price: 0, DurationDays: 30,
	})
	if err != nil {
		t.Fatalf("create free plan: %v", err)
	}

	resp, err := svc.StartCheckout(ctx, tenantID, userID, subscriptions.CheckoutRequest{
		PlanID: uuid.UUID(p.ID.Bytes),
	}, "pk_test")
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	if resp.Amount != 0 {
		t.Fatalf("expected free plan amount=0, got %v", resp.Amount)
	}

	sub, err := svc.GetActive(ctx, userID)
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if sub.Status.String != "active" {
		t.Fatalf("expected free plan to activate immediately, status=%q", sub.Status.String)
	}
}

func TestStartCheckout_PaidPlanWithoutRazorpayFails(t *testing.T) {
	pool := testPool(t)
	svc := subscriptions.NewService(pool, nil) // no Razorpay configured
	tenantID, userID := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	planID := createPlan(t, ctx, svc, tenantID, 499)
	_, err := svc.StartCheckout(ctx, tenantID, userID, subscriptions.CheckoutRequest{PlanID: planID}, "pk_test")
	if err == nil {
		t.Fatal("expected error starting checkout for a paid plan with no Razorpay configured")
	}
}

func TestVerifyCheckout_RejectsInvalidSignature(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", testKeySecret, "")
	svc := subscriptions.NewService(pool, rp)
	tenantID, userID := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	_, err := svc.VerifyCheckout(ctx, userID, subscriptions.VerifyRequest{
		RazorpayOrderID:   "order_x",
		RazorpayPaymentID: "pay_x",
		RazorpaySignature: "not-a-real-signature",
	})
	if err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestVerifyCheckout_SuccessActivatesSubscription(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", testKeySecret, "")
	svc := subscriptions.NewService(pool, rp)
	tenantID, userID := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	p, err := svc.CreatePlan(ctx, tenantID, subscriptions.UpsertPlanRequest{
		Name: "Verify Plan", Slug: "verify-plan-" + userID.String()[:8],
		Price: 999, DurationDays: 30,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planID := uuid.UUID(p.ID.Bytes)

	// Fixture a pending subscription + payment directly — this is what a
	// real StartCheckout would have produced for a paid plan, but without
	// depending on a live Razorpay CreateOrder network call.
	var subID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO user_subscriptions (user_id, plan_id, status, starts_at, ends_at, auto_renew, tenant_id)
		VALUES ($1, $2, 'pending', now(), now(), false, $3)
		RETURNING id
	`, userID, planID, tenantID).Scan(&subID); err != nil {
		t.Fatalf("fixture pending subscription: %v", err)
	}

	orderID := "order_" + uuid.New().String()[:12]
	if _, err := pool.Exec(ctx, `
		INSERT INTO payments (user_id, subscription_id, amount, currency, provider, provider_order_id, status, metadata, tenant_id)
		VALUES ($1, $2, 999, 'INR', 'razorpay', $3, 'created', '{}', $4)
	`, userID, subID, orderID, tenantID); err != nil {
		t.Fatalf("fixture pending payment: %v", err)
	}

	sig := computeSignature(orderID, "pay_verify_x", testKeySecret)
	updated, err := svc.VerifyCheckout(ctx, userID, subscriptions.VerifyRequest{
		RazorpayOrderID:   orderID,
		RazorpayPaymentID: "pay_verify_x",
		RazorpaySignature: sig,
	})
	if err != nil {
		t.Fatalf("VerifyCheckout: %v", err)
	}
	if updated.Status.String != "active" {
		t.Fatalf("expected active status, got %q", updated.Status.String)
	}
}

func TestCancel_RejectsWrongUser(t *testing.T) {
	pool := testPool(t)
	svc := subscriptions.NewService(pool, nil)
	tenantID, userID := fixtures(t, pool)
	attackerID := uuid.New()
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	p, err := svc.CreatePlan(ctx, tenantID, subscriptions.UpsertPlanRequest{
		Name: "Cancel Plan", Slug: "cancel-plan-" + userID.String()[:8],
		Price: 0, DurationDays: 30,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	resp, err := svc.StartCheckout(ctx, tenantID, userID, subscriptions.CheckoutRequest{PlanID: uuid.UUID(p.ID.Bytes)}, "pk_test")
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	subID, err := uuid.Parse(resp.SubscriptionID)
	if err != nil {
		t.Fatalf("parse subscription id: %v", err)
	}

	if err := svc.Cancel(ctx, attackerID, subID); err == nil {
		t.Fatal("expected forbidden error cancelling someone else's subscription")
	}
}

func TestHandleWebhook_RejectsInvalidSignature(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", "sec", "webhook_secret")
	svc := subscriptions.NewService(pool, rp)
	ctx := context.Background()

	err := svc.HandleWebhook(ctx, []byte(`{"event":"payment.captured"}`), "bad-signature")
	if err == nil {
		t.Fatal("expected invalid webhook signature error")
	}
}

func computeSignature(orderID, paymentID, keySecret string) string {
	mac := hmac.New(sha256.New, []byte(keySecret))
	mac.Write([]byte(orderID + "|" + paymentID))
	return hex.EncodeToString(mac.Sum(nil))
}
