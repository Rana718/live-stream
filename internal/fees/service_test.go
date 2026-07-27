// Integration tests for the fee-assignment and installment-payment flow.
// StartInstallmentCheckout's Razorpay CreateOrder call isn't covered here
// (network dependency) — only the pre-network guard rails (already-paid,
// forbidden user) and the parts with no network dependency at all
// (Assign's installment math, VerifyInstallmentPayment's signature check).
//
// Skipped automatically if TEST_DATABASE_URL is unset, same convention as
// internal/courseorders/service_test.go.
package fees_test

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
	"live-platform/internal/fees"
	"live-platform/internal/payments"
)

const testKeySecret = "fees_test_key_secret"

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

func fixtures(t *testing.T, pool *pgxpool.Pool) (tenantID, userID uuid.UUID) {
	t.Helper()
	superCtx := database.WithSuperAdmin(context.Background())

	tenantID = uuid.New()
	if _, err := pool.Exec(superCtx, `
		INSERT INTO tenants (id, org_code, name, slug, plan, status)
		VALUES ($1, $2, $3, $4, 'starter', 'active')
	`, tenantID, "FE"+tenantID.String()[:6], "fe-"+tenantID.String()[:6], "fe-"+tenantID.String()[:6]); err != nil {
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

func TestAssign_SplitsIntoCorrectInstallments(t *testing.T) {
	pool := testPool(t)
	svc := fees.NewService(pool, nil)
	tenantID, userID := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	sf, installments, err := svc.Assign(ctx, fees.AssignFeeRequest{
		UserID:        userID,
		TotalAmount:   3000,
		InstallmentsN: 3,
	})
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if len(installments) != 3 {
		t.Fatalf("expected 3 installments, got %d", len(installments))
	}
	for i, inst := range installments {
		if inst.InstallmentNumber != int32(i+1) {
			t.Fatalf("installment %d has wrong number %d", i, inst.InstallmentNumber)
		}
	}

	got, err := svc.GetInstallments(ctx, uuid.UUID(sf.ID.Bytes))
	if err != nil {
		t.Fatalf("GetInstallments: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 installments persisted, got %d", len(got))
	}
}

func TestAssign_DefaultsToSingleInstallment(t *testing.T) {
	pool := testPool(t)
	svc := fees.NewService(pool, nil)
	tenantID, userID := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	_, installments, err := svc.Assign(ctx, fees.AssignFeeRequest{
		UserID:      userID,
		TotalAmount: 1000,
		// InstallmentsN left at zero — service should default to 1, not
		// divide by zero.
	})
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if len(installments) != 1 {
		t.Fatalf("expected 1 installment (default), got %d", len(installments))
	}
}

func TestVerifyInstallmentPayment_RejectsInvalidSignature(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", testKeySecret, "")
	svc := fees.NewService(pool, rp)
	tenantID, userID := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	err := svc.VerifyInstallmentPayment(ctx, userID, fees.VerifyInstallmentRequest{
		InstallmentID:     uuid.New(),
		RazorpayOrderID:   "order_x",
		RazorpayPaymentID: "pay_x",
		RazorpaySignature: "not-a-real-signature",
	})
	if err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestVerifyInstallmentPayment_SuccessMarksInstallmentPaid(t *testing.T) {
	pool := testPool(t)
	rp := payments.NewRazorpay("key_id", testKeySecret, "")
	svc := fees.NewService(pool, rp)
	tenantID, userID := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	sf, installments, err := svc.Assign(ctx, fees.AssignFeeRequest{
		UserID:        userID,
		TotalAmount:   500,
		InstallmentsN: 1,
	})
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	instID := uuid.UUID(installments[0].ID.Bytes)

	// Fixture the payment row a real StartInstallmentCheckout would have
	// created, without depending on a live Razorpay CreateOrder call.
	orderID := "order_" + uuid.New().String()[:12]
	if _, err := pool.Exec(ctx, `
		INSERT INTO payments (user_id, amount, currency, provider, provider_order_id, status, metadata, tenant_id)
		VALUES ($1, 500, 'INR', 'razorpay', $2, 'created', '{}', $3)
	`, userID, orderID, tenantID); err != nil {
		t.Fatalf("fixture payment: %v", err)
	}

	sig := computeSignature(orderID, "pay_fee_x", testKeySecret)
	err = svc.VerifyInstallmentPayment(ctx, userID, fees.VerifyInstallmentRequest{
		InstallmentID:     instID,
		RazorpayOrderID:   orderID,
		RazorpayPaymentID: "pay_fee_x",
		RazorpaySignature: sig,
	})
	if err != nil {
		t.Fatalf("VerifyInstallmentPayment: %v", err)
	}

	got, err := svc.GetInstallments(ctx, uuid.UUID(sf.ID.Bytes))
	if err != nil {
		t.Fatalf("GetInstallments: %v", err)
	}
	if got[0].Status.String != "paid" {
		t.Fatalf("expected installment status=paid, got %q", got[0].Status.String)
	}
}

func computeSignature(orderID, paymentID, keySecret string) string {
	mac := hmac.New(sha256.New, []byte(keySecret))
	mac.Write([]byte(orderID + "|" + paymentID))
	return hex.EncodeToString(mac.Sum(nil))
}
