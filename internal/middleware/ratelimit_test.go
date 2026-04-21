package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
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
