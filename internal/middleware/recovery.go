package middleware

import (
	"log/slog"
	"runtime/debug"

	"github.com/gofiber/fiber/v3"
)

// Recovery catches any panic in downstream handlers, logs it with the full
// stack trace plus request context, and returns a 500 so the process keeps running.
func Recovery(l *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				reqID, _ := c.Locals("request_id").(string)
				l.Error("panic recovered",
					"request_id", reqID,
					"method", c.Method(),
					"path", c.Path(),
					"panic", r,
					"stack", stack,
				)
				err = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error":      "internal server error",
					"request_id": reqID,
				})
			}
		}()
		return c.Next()
	}
}
