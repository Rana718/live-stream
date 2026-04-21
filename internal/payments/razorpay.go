package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Razorpay is a minimal REST client for the Razorpay Orders API.
// Docs: https://razorpay.com/docs/api/orders
type Razorpay struct {
	keyID         string
	keySecret     string
	webhookSecret string
	endpoint      string
	http          *http.Client
}

func NewRazorpay(keyID, keySecret, webhookSecret string) *Razorpay {
	return &Razorpay{
		keyID:         keyID,
		keySecret:     keySecret,
		webhookSecret: webhookSecret,
		endpoint:      "https://api.razorpay.com/v1",
		http:          &http.Client{Timeout: 20 * time.Second},
	}
}

type Order struct {
	ID       string `json:"id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
	Receipt  string `json:"receipt"`
}

type createOrderReq struct {
	Amount   int64             `json:"amount"`
	Currency string            `json:"currency"`
	Receipt  string            `json:"receipt,omitempty"`
	Notes    map[string]string `json:"notes,omitempty"`
}

// CreateOrder creates a Razorpay order. Amount must be in the smallest unit (paise for INR).
func (r *Razorpay) CreateOrder(ctx context.Context, amountPaise int64, currency, receipt string, notes map[string]string) (*Order, error) {
	if r.keyID == "" || r.keySecret == "" {
		return nil, fmt.Errorf("razorpay keys not configured")
	}
	body := createOrderReq{Amount: amountPaise, Currency: currency, Receipt: receipt, Notes: notes}
	buf, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint+"/orders", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(r.keyID, r.keySecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("razorpay http %d: %s", resp.StatusCode, string(raw))
	}
	var out Order
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// VerifyPaymentSignature verifies checkout signatures (orderID|paymentID signed with keySecret).
func (r *Razorpay) VerifyPaymentSignature(orderID, paymentID, signature string) bool {
	if r.keySecret == "" {
		return false
	}
	payload := orderID + "|" + paymentID
	mac := hmac.New(sha256.New, []byte(r.keySecret))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// VerifyWebhookSignature verifies X-Razorpay-Signature for webhook payloads.
func (r *Razorpay) VerifyWebhookSignature(body []byte, signature string) bool {
	if r.webhookSecret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(r.webhookSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
