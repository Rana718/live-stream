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
	Amount    int64             `json:"amount"`
	Currency  string            `json:"currency"`
	Receipt   string            `json:"receipt,omitempty"`
	Notes     map[string]string `json:"notes,omitempty"`
	Transfers []Transfer        `json:"transfers,omitempty"`
}

// Transfer is a Razorpay-Route split: a portion of the order amount that
// flows to a Linked Account. The platform's commission is whatever's left
// after summing all transfers.
//
// Docs: https://razorpay.com/docs/payments/payments/route/
type Transfer struct {
	Account  string            `json:"account"`            // tenant Linked Account ID (acc_XXX)
	Amount   int64             `json:"amount"`             // paise to forward
	Currency string            `json:"currency"`           // typically "INR"
	Notes    map[string]string `json:"notes,omitempty"`
	OnHold   bool              `json:"on_hold,omitempty"`  // hold settlement
}

// CreateOrder creates a Razorpay order. Amount must be in the smallest unit (paise for INR).
func (r *Razorpay) CreateOrder(ctx context.Context, amountPaise int64, currency, receipt string, notes map[string]string) (*Order, error) {
	return r.CreateOrderWithTransfers(ctx, amountPaise, currency, receipt, notes, nil)
}

// CreateOrderWithTransfers is the Razorpay-Route path. Pass a non-nil
// `transfers` slice to have Razorpay auto-split settlement to tenant
// Linked Accounts on capture. The sum of transfer amounts must be < the
// order amount; the remainder is the platform's commission.
//
// If `transfers` is nil the call falls through to a plain order — no Route
// account required, useful while a tenant is still onboarding KYC.
func (r *Razorpay) CreateOrderWithTransfers(
	ctx context.Context,
	amountPaise int64,
	currency, receipt string,
	notes map[string]string,
	transfers []Transfer,
) (*Order, error) {
	if r.keyID == "" || r.keySecret == "" {
		return nil, fmt.Errorf("razorpay keys not configured")
	}
	body := createOrderReq{
		Amount:    amountPaise,
		Currency:  currency,
		Receipt:   receipt,
		Notes:     notes,
		Transfers: transfers,
	}
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

// LinkedAccount mirrors the fields we care about from the Accounts API.
type LinkedAccount struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	ContactName   string `json:"contact_name"`
	BusinessType  string `json:"business_type"`
}

type createLinkedAccountReq struct {
	Email             string `json:"email"`
	Phone             string `json:"phone,omitempty"`
	Type              string `json:"type"`
	LegalBusinessName string `json:"legal_business_name"`
	BusinessType      string `json:"business_type"`
	ContactName       string `json:"contact_name"`
	ReferenceID       string `json:"reference_id,omitempty"`
}

// CreateLinkedAccountInput is what the super_admin UI sends.
type CreateLinkedAccountInput struct {
	Email             string `json:"email" validate:"required,email"`
	Phone             string `json:"phone"`
	LegalBusinessName string `json:"legal_business_name" validate:"required"`
	BusinessType      string `json:"business_type" validate:"required"`
	ContactName       string `json:"contact_name" validate:"required"`
	ReferenceID       string `json:"reference_id"`
}

// CreateLinkedAccount provisions a Route Linked Account. Razorpay still
// requires the tenant to complete KYC + bank verification in their
// dashboard; this just kicks off the record so support doesn't have to
// context-switch out of /super.
//
// Docs: https://razorpay.com/docs/api/partners/account-onboarding/
func (r *Razorpay) CreateLinkedAccount(ctx context.Context, in CreateLinkedAccountInput) (*LinkedAccount, error) {
	if r.keyID == "" || r.keySecret == "" {
		return nil, fmt.Errorf("razorpay keys not configured")
	}
	body := createLinkedAccountReq{
		Email:             in.Email,
		Phone:             in.Phone,
		Type:              "route",
		LegalBusinessName: in.LegalBusinessName,
		BusinessType:      in.BusinessType,
		ContactName:       in.ContactName,
		ReferenceID:       in.ReferenceID,
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint+"/accounts", bytes.NewReader(buf))
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
		return nil, fmt.Errorf("razorpay accounts %d: %s", resp.StatusCode, string(raw))
	}
	var out LinkedAccount
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
