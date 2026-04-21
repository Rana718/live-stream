package payments

import "testing"

func TestVerifyPaymentSignatureRejectsInvalid(t *testing.T) {
	r := NewRazorpay("key_id", "key_secret", "webhook_secret")
	if r.VerifyPaymentSignature("order_1", "pay_1", "not-a-real-signature") {
		t.Fatal("expected invalid signature to fail")
	}
}

func TestVerifyPaymentSignatureAcceptsComputed(t *testing.T) {
	// Pre-computed HMAC-SHA256("order_X|pay_Y", "secret") in hex:
	// echo -n "order_X|pay_Y" | openssl dgst -sha256 -hmac "secret"
	// → bff0253d929e5412fff37172ea2b01ce8c27545979cf78d688763d7eb9307cef
	r := NewRazorpay("key_id", "secret", "")
	if !r.VerifyPaymentSignature("order_X", "pay_Y",
		"bff0253d929e5412fff37172ea2b01ce8c27545979cf78d688763d7eb9307cef") {
		t.Fatal("expected pre-computed signature to verify")
	}
}

func TestVerifyWebhookEmptySecretFails(t *testing.T) {
	r := NewRazorpay("k", "s", "")
	if r.VerifyWebhookSignature([]byte("{}"), "anything") {
		t.Fatal("webhook verification must fail without configured secret")
	}
}
