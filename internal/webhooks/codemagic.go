package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"

	"live-platform/internal/appbuilds"
	"live-platform/internal/config"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// CodemagicHandler closes the build loop. When the per-tenant build
// finishes (success or failure) Codemagic POSTs back here with the build
// status and artifact URLs; we patch the corresponding app_builds row so
// the support UI shows it published without a manual click.
//
// Codemagic's webhook signing is a SHA-256 HMAC of the raw body using a
// shared secret configured in the Codemagic dashboard. If
// CODEMAGIC_WEBHOOK_SECRET is unset we still accept calls (so dev / first
// integrations work) but log loudly so production doesn't run unguarded.
type CodemagicHandler struct {
	svc *appbuilds.Service
	cfg config.CodemagicConfig
	log *slog.Logger
}

func NewCodemagicHandler(svc *appbuilds.Service, cfg config.CodemagicConfig, log *slog.Logger) *CodemagicHandler {
	return &CodemagicHandler{svc: svc, cfg: cfg, log: log}
}

type codemagicWebhook struct {
	Build struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		// We carry our internal build_id in the workflow's environment vars;
		// Codemagic echoes them back on the webhook payload so we can map
		// without remembering the Codemagic build ID on our side.
		Environment map[string]string `json:"environment"`
	} `json:"build"`
	Artifacts []struct {
		URL  string `json:"url"`
		Name string `json:"name"`
	} `json:"artifacts"`
	Error string `json:"error_message,omitempty"`
}

// Receive handles POST /api/v1/webhooks/codemagic.
func (h *CodemagicHandler) Receive(c fiber.Ctx) error {
	body, _ := io.ReadAll(c.Request().BodyStream())
	if len(body) == 0 {
		body = c.Body()
	}

	if h.cfg.WebhookSecret != "" {
		got := c.Get("X-Cm-Signature")
		if !verifyHMAC(body, got, h.cfg.WebhookSecret) {
			h.log.Warn("codemagic webhook signature mismatch")
			return c.SendStatus(fiber.StatusUnauthorized)
		}
	} else {
		h.log.Warn("codemagic webhook secret unset — running unauthenticated")
	}

	var env codemagicWebhook
	if err := json.Unmarshal(body, &env); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	internalID, ok := env.Build.Environment["BUILD_ID"]
	if !ok {
		// Webhook for a build we didn't trigger — ack and ignore.
		return c.SendStatus(fiber.StatusOK)
	}
	id, err := uuid.Parse(internalID)
	if err != nil {
		return c.SendStatus(fiber.StatusOK)
	}

	// Codemagic uses statuses like 'finished', 'building', 'failed'. Map to
	// our enum.
	status := "building"
	switch env.Build.Status {
	case "finished":
		status = "published"
	case "failed", "canceled":
		status = "failed"
	}

	buildURL := ""
	for _, a := range env.Artifacts {
		// Pick the first AAB / IPA we see; Codemagic returns the full set.
		if a.Name != "" {
			buildURL = a.URL
			break
		}
	}

	if _, err := h.svc.SetStatus(c.Context(), id, appbuilds.SetStatusInput{
		Status:   status,
		BuildURL: buildURL,
		ErrorLog: env.Error,
	}); err != nil {
		h.log.Error("codemagic webhook patch failed",
			slog.String("build_id", id.String()),
			slog.String("err", err.Error()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}

func verifyHMAC(body []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(signature))
}
