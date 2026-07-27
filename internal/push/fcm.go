// Package push wraps a push-notification provider behind a Client
// interface. Default implementation is FCM HTTP v1, authenticated with a
// service-account OAuth2 token — the legacy server-key API this used to
// call was retired by Google in 2024, so there's no fallback to it.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
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

// FCM implements Client against FCM HTTP v1. Unlike the legacy API, v1 has
// no multicast endpoint — every token needs its own request — so Send fans
// out over a bounded worker pool instead of one batched call.
type FCM struct {
	projectID      string
	tokens         *tokenSource
	http           *http.Client
	baseURL        string
	maxConcurrency int
	log            *slog.Logger
}

// New builds an FCM v1 client from a service-account key sourced from
// config (inline JSON or a mounted file). Returns nil (disabled) if push
// isn't configured or the key fails to parse — callers must handle a nil
// Client by degrading to in-app-only notifications.
func New(cfg config.PushConfig, log *slog.Logger) Client {
	if strings.ToLower(cfg.Provider) != "fcm" {
		return nil
	}
	rawJSON, err := loadServiceAccountJSON(cfg)
	if err != nil {
		log.Error("fcm disabled — service account load failed", slog.String("err", err.Error()))
		return nil
	}

	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 6 * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}

	ts, err := newTokenSource(rawJSON, httpClient)
	if err != nil {
		log.Error("fcm disabled — invalid service account json", slog.String("err", err.Error()))
		return nil
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://fcm.googleapis.com"
	}
	maxConcurrency := cfg.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 20
	}
	return &FCM{
		projectID:      ts.key.ProjectID,
		tokens:         ts,
		http:           httpClient,
		baseURL:        baseURL,
		maxConcurrency: maxConcurrency,
		log:            log,
	}
}

func loadServiceAccountJSON(cfg config.PushConfig) ([]byte, error) {
	if cfg.ServiceAccountJSON != "" {
		return []byte(cfg.ServiceAccountJSON), nil
	}
	if cfg.ServiceAccountFile != "" {
		return os.ReadFile(cfg.ServiceAccountFile)
	}
	return nil, errors.New("neither FCM_SERVICE_ACCOUNT_JSON nor FCM_SERVICE_ACCOUNT_FILE set")
}

type fcmV1Envelope struct {
	Message fcmV1Message `json:"message"`
}

type fcmV1Message struct {
	Token        string            `json:"token"`
	Notification fcmV1Notification `json:"notification"`
	Data         map[string]string `json:"data,omitempty"`
	Android      *fcmAndroidConfig `json:"android,omitempty"`
}

type fcmV1Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type fcmAndroidConfig struct {
	Priority string `json:"priority"`
}

// Send dispatches to every token concurrently (bounded by maxConcurrency)
// since v1 has no multicast endpoint. Callers here only ever pass one
// user's own device tokens (a handful), so this stays cheap in practice —
// the pool just protects against an accidental large fan-out.
//
// Returns an error only if every send failed; partial failures (a stale
// token mixed into a live one) are logged, not surfaced, since the caller
// can't act on a per-token result anyway.
func (f *FCM) Send(ctx context.Context, tokens []string, n Notification) error {
	if len(tokens) == 0 {
		return nil
	}

	sem := make(chan struct{}, f.maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := 0
	var lastErr error

	for _, tok := range tokens {
		wg.Add(1)
		sem <- struct{}{}
		go func(token string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := f.sendOne(ctx, token, n); err != nil {
				mu.Lock()
				failed++
				lastErr = err
				mu.Unlock()
			}
		}(tok)
	}
	wg.Wait()

	if failed > 0 {
		f.log.Warn("fcm partial failure",
			slog.Int("failed", failed),
			slog.Int("total", len(tokens)))
	}
	if failed == len(tokens) {
		return fmt.Errorf("fcm: all %d sends failed, last error: %w", len(tokens), lastErr)
	}
	f.log.Info("fcm sent",
		slog.Int("token_count", len(tokens)-failed),
		slog.String("title", n.Title))
	return nil
}

func (f *FCM) sendOne(ctx context.Context, token string, n Notification) error {
	accessToken, err := f.tokens.Token(ctx)
	if err != nil {
		return fmt.Errorf("mint access token: %w", err)
	}

	envelope := fcmV1Envelope{Message: fcmV1Message{
		Token:        token,
		Notification: fcmV1Notification{Title: n.Title, Body: n.Body},
		Data:         n.Data,
		Android:      &fcmAndroidConfig{Priority: "high"},
	}}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/v1/projects/%s/messages:send", f.baseURL, f.projectID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)

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
	return nil
}
