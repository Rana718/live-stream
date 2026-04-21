package middleware

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// AuditRecorder is the minimal interface the middleware uses so we don't import the audit package
// (avoiding a cycle between middleware and audit).
type AuditRecorder interface {
	Write(ctx context.Context, actorID uuid.UUID, actorRole, action, resourceType string,
		resourceID *uuid.UUID, ip, userAgent string, metadata map[string]any) error
}

// Audit wraps a handler chain so that every request with a mutating method (POST/PUT/DELETE)
// performed by an authenticated user produces an audit log entry. Async best-effort.
func Audit(rec AuditRecorder) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()

		method := c.Method()
		if method != fiber.MethodPost && method != fiber.MethodPut && method != fiber.MethodDelete && method != fiber.MethodPatch {
			return err
		}
		uid, ok := c.Locals("userID").(uuid.UUID)
		if !ok {
			return err
		}
		role, _ := c.Locals("role").(string)

		action := method + " " + c.Route().Path
		ip := c.IP()
		ua := c.Get("User-Agent")
		status := c.Response().StatusCode()

		// Fire-and-forget. We don't hold up the response or surface audit-log errors.
		bgCtx := context.Background()
		go func() {
			_ = rec.Write(bgCtx, uid, role, action, "", nil, ip, ua, map[string]any{
				"status": status,
			})
		}()
		return err
	}
}
