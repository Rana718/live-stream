// Package whatsapp wraps a WhatsApp provider behind a Client interface.
// Default implementation hits Gupshup's text-message API. Plug in any
// other provider (360dialog, Twilio) by satisfying the Client contract.
package whatsapp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"live-platform/internal/config"
)

// Client is what notification + broadcast services depend on. Keeping it
// narrow lets us swap providers without changing callers.
type Client interface {
	Send(ctx context.Context, to, message string) error
	Broadcast(ctx context.Context, recipients []string, message string) (sent int, err error)
}

type Gupshup struct {
	cfg  config.WhatsAppConfig
	http *http.Client
	log  *slog.Logger
}

// New picks the right impl off the config. nil = disabled (broadcast endpoint
// returns a clear error). Drops loudly into the log if a half-configured
// state is detected so production deploys don't ship silently broken.
func New(cfg config.WhatsAppConfig, log *slog.Logger) Client {
	if strings.ToLower(cfg.Provider) != "gupshup" || cfg.APIKey == "" {
		if cfg.Provider != "" {
			log.Warn("whatsapp partially configured — disabling",
				slog.String("provider", cfg.Provider),
				slog.Bool("has_key", cfg.APIKey != ""))
		}
		return nil
	}
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 8 * time.Second
	}
	return &Gupshup{cfg: cfg, http: &http.Client{Timeout: timeout}, log: log}
}

// Send delivers a single text message. We use Gupshup's form-encoded API
// rather than the JSON one so the integration works with both Sandbox and
// production tier accounts without a tier flag.
func (g *Gupshup) Send(ctx context.Context, to, message string) error {
	to = strings.TrimPrefix(strings.TrimSpace(to), "+")
	if to == "" {
		return fmt.Errorf("missing recipient")
	}

	form := url.Values{}
	form.Set("channel", "whatsapp")
	form.Set("source", g.cfg.Source)
	form.Set("destination", to)
	form.Set("src.name", g.cfg.AppName)
	form.Set("message", fmt.Sprintf(`{"type":"text","text":%q}`, message))

	endpoint := strings.TrimRight(g.cfg.BaseURL, "/") + "/msg"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("apikey", g.cfg.APIKey)

	resp, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("gupshup dispatch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		g.log.Error("gupshup send failed",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(raw)))
		return fmt.Errorf("gupshup status %d", resp.StatusCode)
	}
	return nil
}

// Broadcast fans out to every recipient sequentially. Gupshup's per-account
// throughput cap is well above what a single tenant typically needs (a few
// thousand recipients), so sequential is fine; for >10k we'd batch in
// goroutine pool — left as a Phase-5 optimisation.
func (g *Gupshup) Broadcast(ctx context.Context, recipients []string, message string) (int, error) {
	sent := 0
	for _, r := range recipients {
		if ctx.Err() != nil {
			return sent, ctx.Err()
		}
		if err := g.Send(ctx, r, message); err != nil {
			// Don't abort — log and keep going so a single bad number
			// doesn't drop the whole campaign.
			g.log.Warn("whatsapp broadcast skip",
				slog.String("to", r),
				slog.String("err", err.Error()))
			continue
		}
		sent++
	}
	return sent, nil
}
