package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

// RateLimit implements a simple per-IP token-bucket rate limiter.
// rpm = requests per minute, burst = maximum concurrent tokens.
// Uses Redis-free in-memory storage; swap for Redis in multi-node deployments.
func RateLimit(rpm, burst int) fiber.Handler {
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
		key := c.IP()
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
