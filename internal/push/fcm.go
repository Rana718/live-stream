// Package push wraps a push-notification provider behind a Client interface.
// Default implementation hits FCM legacy HTTP. To switch to FCM HTTP v1,
// swap the auth header for an OAuth bearer minted from the service account
// JSON; the request shape is otherwise identical.
package push

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

// Notification carries the user-visible content. Data is forwarded to the
// app as a flat string→string map (FCM constraint) so the app can deep-link
// or update local state without rendering the title/body itself.
type Notification struct {
	Title string
	Body  string
	Data  map[string]string
}

type Client interface {
	Send(ctx context.Context, tokens []string, n Notification) error
}

type FCM struct {
	cfg  config.PushConfig
	http *http.Client
	log  *slog.Logger
}

func New(cfg config.PushConfig, log *slog.Logger) Client {
	if strings.ToLower(cfg.Provider) != "fcm" || cfg.ServerKey == "" {
		return nil
	}
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 6 * time.Second
	}
	return &FCM{cfg: cfg, http: &http.Client{Timeout: timeout}, log: log}
}

type fcmReq struct {
	RegistrationIDs []string          `json:"registration_ids,omitempty"`
	To              string            `json:"to,omitempty"`
	Notification    fcmNotification   `json:"notification"`
	Data            map[string]string `json:"data,omitempty"`
	Priority        string            `json:"priority"`
}

type fcmNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Sound string `json:"sound"`
}

// Send dispatches one push to up to 1000 tokens (FCM limit). For larger
// fan-outs the caller should chunk and call Send per chunk so a single
// failure doesn't drop the whole batch.
func (f *FCM) Send(ctx context.Context, tokens []string, n Notification) error {
	if len(tokens) == 0 {
		return nil
	}
	if len(tokens) > 1000 {
		tokens = tokens[:1000]
		f.log.Warn("fcm batch capped at 1000 tokens")
	}
	req := fcmReq{
		RegistrationIDs: tokens,
		Notification: fcmNotification{
			Title: n.Title,
			Body:  n.Body,
			Sound: "default",
		},
		Data:     n.Data,
		Priority: "high",
	}
	if len(tokens) == 1 {
		req.To = tokens[0]
		req.RegistrationIDs = nil
	}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, f.cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "key="+f.cfg.ServerKey)

	resp, err := f.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("fcm dispatch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		f.log.Error("fcm send failed",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(raw)))
		return fmt.Errorf("fcm status %d", resp.StatusCode)
	}
	f.log.Info("fcm sent",
		slog.Int("token_count", len(tokens)),
		slog.String("title", n.Title))
	return nil
}
