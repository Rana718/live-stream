// Package sms wraps an SMS provider behind a Client interface so the auth
// service can stay provider-agnostic. The default implementation talks to
// MSG91 (Indian DLT-compliant). Plug in any other provider by satisfying
// the same Client contract and wiring it in main.go.
package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"live-platform/internal/config"
)

// Client is the contract auth uses for OTP delivery. We deliberately keep it
// narrow — adding more methods later (transactional templates, bulk push)
// shouldn't bloat callers.
type Client interface {
	SendOTP(ctx context.Context, phone, code string) error
}

// MSG91 implements Client by hitting MSG91's v5 OTP endpoint.
//
// Endpoint reference: https://docs.msg91.com/p/tf9GTextN/e/A5vkdqfbu/MSG91
// The SendOTP we use POSTs JSON to /otp with the code embedded as a template
// variable so MSG91 picks the DLT-approved template by ID.
type MSG91 struct {
	cfg    config.SMSConfig
	http   *http.Client
	log    *slog.Logger
}

// New picks the right implementation off the config. If the provider is
// blank or the AuthKey is missing, returns nil so callers can degrade
// gracefully (dev mode logs the OTP instead of sending it).
func New(cfg config.SMSConfig, log *slog.Logger) Client {
	if strings.ToLower(cfg.Provider) != "msg91" || cfg.AuthKey == "" {
		return nil
	}
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 8 * time.Second
	}
	return &MSG91{
		cfg:  cfg,
		http: &http.Client{Timeout: timeout},
		log:  log,
	}
}

type msg91Req struct {
	TemplateID string            `json:"template_id"`
	Mobile     string            `json:"mobile"`
	SenderID   string            `json:"sender,omitempty"`
	OTP        string            `json:"otp"`
	Variables  map[string]string `json:"variables_values,omitempty"`
}

type msg91Resp struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// SendOTP fires the request once. We don't loop here — auth callers can
// retry idempotently because each (phone, code) pair lives in our own
// sms_codes table; double-sending would just cost an SMS without breaking
// state. Add a backoff retry layer here if MSG91 outages become common.
func (m *MSG91) SendOTP(ctx context.Context, phone, code string) error {
	mobile := strings.TrimPrefix(phone, "+")
	body := msg91Req{
		TemplateID: m.cfg.OTPTemplate,
		Mobile:     mobile,
		SenderID:   m.cfg.SenderID,
		OTP:        code,
		Variables:  map[string]string{"otp": code},
	}
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}

	url := strings.TrimRight(m.cfg.BaseURL, "/") + "/otp"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authkey", m.cfg.AuthKey)

	resp, err := m.http.Do(req)
	if err != nil {
		return fmt.Errorf("msg91 dispatch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		m.log.Error("msg91 send failed",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(raw)),
			slog.String("phone_suffix", lastFour(mobile)))
		return fmt.Errorf("msg91 status %d", resp.StatusCode)
	}

	var parsed msg91Resp
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	m.log.Info("msg91 otp sent",
		slog.String("phone_suffix", lastFour(mobile)),
		slog.String("type", parsed.Type))
	return nil
}

// lastFour redacts all but the last four digits so logs don't accidentally
// dox the user's phone number.
func lastFour(phone string) string {
	if len(phone) <= 4 {
		return phone
	}
	return strings.Repeat("*", len(phone)-4) + phone[len(phone)-4:]
}
