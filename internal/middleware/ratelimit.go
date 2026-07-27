package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// RateLimit implements a simple per-IP token-bucket rate limiter.
// rpm = requests per minute, burst = maximum concurrent tokens.
// Uses Redis-free in-memory storage; swap for Redis in multi-node deployments.
//
// This alone isn't tenant-aware: several tenants' users sharing a NAT/IP
// pool (a school's shared campus WiFi, a mobile carrier's CGNAT) bucket
// together, so one tenant's traffic spike can throttle another's. Mount
// RateLimitPerTenant as well on authenticated routes for a second,
// tenant-keyed bucket — see its doc comment.
func RateLimit(rpm, burst int) fiber.Handler {
	return tokenBucketLimiter(rpm, burst, func(c fiber.Ctx) string {
		return "ip:" + c.IP()
	})
}

// RateLimitPerTenant is a second, independently-budgeted token bucket
// keyed by tenant ID instead of IP. Mount it after TenantContext (or
// anything else that sets c.Locals("tenantID")) on routes where you want
// one tenant's traffic spike (a viral test, a mass live-class join) to
// throttle only that tenant, not bucket-mates sharing an IP/NAT with
// unrelated tenants. Requests with no tenant in context (not yet
// authenticated) fall through unlimited here — RateLimit's per-IP bucket
// already covers that surface.
func RateLimitPerTenant(rpm, burst int) fiber.Handler {
	limiter := tokenBucketLimiter(rpm, burst, func(c fiber.Ctx) string {
		tenantID, ok := c.Locals("tenantID").(uuid.UUID)
		if !ok || tenantID == uuid.Nil {
			return ""
		}
		return "tenant:" + tenantID.String()
	})
	return func(c fiber.Ctx) error {
		if _, ok := c.Locals("tenantID").(uuid.UUID); !ok {
			return c.Next()
		}
		return limiter(c)
	}
}

// tokenBucketLimiter is the shared token-bucket implementation behind
// RateLimit and RateLimitPerTenant — same algorithm, different key.
func tokenBucketLimiter(rpm, burst int, keyFunc func(fiber.Ctx) string) fiber.Handler {
	if rpm <= 0 {
		rpm = 120
	}
	if burst <= 0 {
		burst = rpm / 2
	}
	refillPerSec := float64(rpm) / 60.0

	type bucket struct {
		tokens   float64
		lastSeen time.Time
	}

	var (
		mu      sync.Mutex
		buckets = make(map[string]*bucket)
	)

	// Janitor: evict stale entries every 5m.
	go func() {
		tick := time.NewTicker(5 * time.Minute)
		defer tick.Stop()
		for range tick.C {
			mu.Lock()
			cutoff := time.Now().Add(-15 * time.Minute)
			for k, b := range buckets {
				if b.lastSeen.Before(cutoff) {
					delete(buckets, k)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c fiber.Ctx) error {
		key := keyFunc(c)
		if key == "" {
			return c.Next()
		}
		now := time.Now()

		mu.Lock()
		b, ok := buckets[key]
		if !ok {
			b = &bucket{tokens: float64(burst), lastSeen: now}
			buckets[key] = b
		}
		elapsed := now.Sub(b.lastSeen).Seconds()
		b.tokens += elapsed * refillPerSec
		if b.tokens > float64(burst) {
			b.tokens = float64(burst)
		}
		b.lastSeen = now

		if b.tokens < 1 {
			mu.Unlock()
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded",
			})
		}
		b.tokens--
		mu.Unlock()

		return c.Next()
	}
}
