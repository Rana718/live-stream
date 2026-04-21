package middleware

import "github.com/gofiber/fiber/v3"

// SecurityHeaders applies a conservative set of HTTP headers for production use.
// CSP deliberately excludes inline scripts from app responses; Swagger-UI serves
// its own HTML with its own inline script, so we scope CSP to /api/* only.
func SecurityHeaders(tlsEnabled bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("X-XSS-Protection", "0")
		c.Set("Permissions-Policy", "geolocation=(self), microphone=(), camera=()")
		if tlsEnabled {
			c.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}
		return c.Next()
	}
}
