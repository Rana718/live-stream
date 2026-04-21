package middleware

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
)

// RequestLogger emits structured per-request logs using slog.
func RequestLogger(l *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)

		status := c.Response().StatusCode()
		attrs := []any{
			"method", c.Method(),
			"path", c.Path(),
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"ip", c.IP(),
			"ua", c.Get("User-Agent"),
		}
		if uid, ok := c.Locals("userID").(string); ok && uid != "" {
			attrs = append(attrs, "user_id", uid)
		}

		switch {
		case status >= 500:
			l.Error("http_request", attrs...)
		case status >= 400:
			l.Warn("http_request", attrs...)
		default:
			l.Info("http_request", attrs...)
		}
		return err
	}
}
