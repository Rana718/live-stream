package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

func TestRateLimitAllowsUnderBurst(t *testing.T) {
	app := fiber.New()
	app.Use(RateLimit(600, 5)) // burst = 5 initial tokens
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		if err != nil {
			t.Fatalf("req %d error: %v", i, err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("req %d: got %d, want 200", i, resp.StatusCode)
		}
	}
}

func TestRateLimitBlocksBurst(t *testing.T) {
	app := fiber.New()
	app.Use(RateLimit(60, 2))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	// 2 allowed, 3rd should be 429.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		if resp, _ := app.Test(req, fiber.TestConfig{Timeout: 0}); resp.StatusCode != 200 {
			t.Fatalf("req %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
	req := httptest.NewRequest("GET", "/", nil)
	resp, _ := app.Test(req, fiber.TestConfig{Timeout: 0})
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Errorf("got %d, want 429", resp.StatusCode)
	}
}

func TestRateLimitPerTenant_SkipsWhenNoTenantInContext(t *testing.T) {
	app := fiber.New()
	app.Use(RateLimitPerTenant(60, 1))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	// No tenantID ever set in Locals — every request should pass through
	// unlimited (the per-IP RateLimit covers unauthenticated traffic).
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		if err != nil {
			t.Fatalf("req %d error: %v", i, err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("req %d: got %d, want 200 (no tenant context = unlimited here)", i, resp.StatusCode)
		}
	}
}

func TestRateLimitPerTenant_BudgetIsPerTenantNotPerIP(t *testing.T) {
	app := fiber.New()
	tenantA, tenantB := uuid.New(), uuid.New()
	app.Use(func(c fiber.Ctx) error {
		if c.Get("X-Test-Tenant") == "a" {
			c.Locals("tenantID", tenantA)
		} else {
			c.Locals("tenantID", tenantB)
		}
		return c.Next()
	})
	app.Use(RateLimitPerTenant(60, 2))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	// Exhaust tenant A's burst of 2.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Test-Tenant", "a")
		if resp, _ := app.Test(req, fiber.TestConfig{Timeout: 0}); resp.StatusCode != 200 {
			t.Fatalf("tenant A req %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Test-Tenant", "a")
	if resp, _ := app.Test(req, fiber.TestConfig{Timeout: 0}); resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("tenant A 3rd req: expected 429, got %d", resp.StatusCode)
	}

	// Tenant B has its own, untouched budget — same IP, different tenant.
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Test-Tenant", "b")
	if resp, _ := app.Test(req, fiber.TestConfig{Timeout: 0}); resp.StatusCode != 200 {
		t.Fatalf("tenant B req: expected 200 (separate budget), got %d", resp.StatusCode)
	}
}
