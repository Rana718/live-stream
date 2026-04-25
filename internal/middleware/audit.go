package middleware

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// AuditRecorder is the minimal interface the middleware uses so we don't
// import the audit package (avoids cycle between middleware and audit).
//
// The tenantID parameter was added so multi-tenant deployments can scope
// the audit log per tenant.
type AuditRecorder interface {
	Write(ctx context.Context, tenantID, actorID uuid.UUID, actorRole, action, resourceType string,
		resourceID *uuid.UUID, ip, userAgent string, metadata map[string]any) error
}

// Audit wraps a handler chain so every mutating request (POST/PUT/PATCH/DELETE)
// produces an audit row. Async best-effort: we don't hold the response.
//
// Mounted at the route-group level (admin, instructor, etc.) — not globally —
// so noisy student GET routes don't bloat the table.
func Audit(rec AuditRecorder) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()

		method := c.Method()
		if method != fiber.MethodPost && method != fiber.MethodPut &&
			method != fiber.MethodDelete && method != fiber.MethodPatch {
			return err
		}
		uid, ok := c.Locals("userID").(uuid.UUID)
		if !ok {
			return err
		}
		role, _ := c.Locals("role").(string)
		tenantID, _ := c.Locals("tenantID").(uuid.UUID)

		action := method + " " + c.Route().Path
		ip := c.IP()
		ua := c.Get("User-Agent")
		status := c.Response().StatusCode()
		reqID, _ := c.Locals("requestID").(string)

		// context.Background so the audit insert outlives the request ctx
		// (fiber cancels c.Context on response completion).
		bgCtx := context.Background()
		go func() {
			_ = rec.Write(bgCtx, tenantID, uid, role, action, "", nil, ip, ua, map[string]any{
				"status":     status,
				"request_id": reqID,
			})
		}()
		return err
	}
}
