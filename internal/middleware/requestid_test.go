package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestRequestIDGeneratesWhenMissing(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(c.Locals("request_id").(string))
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	if err != nil {
		t.Fatalf("test error: %v", err)
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID header to be set")
	}
}

func TestRequestIDPreservesInbound(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(c.Locals("request_id").(string))
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "inbound-123")
	resp, _ := app.Test(req, fiber.TestConfig{Timeout: 0})
	if got := resp.Header.Get("X-Request-ID"); got != "inbound-123" {
		t.Errorf("got %q, want inbound-123", got)
	}
}
