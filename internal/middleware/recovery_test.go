package middleware

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestRecoveryCatchesPanic(t *testing.T) {
	app := fiber.New()
	silent := slog.New(slog.NewTextHandler(io.Discard, nil))
	app.Use(Recovery(silent))
	app.Get("/boom", func(c fiber.Ctx) error {
		panic("simulated failure")
	})

	req := httptest.NewRequest("GET", "/boom", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	if err != nil {
		t.Fatalf("test error: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Errorf("got %d, want 500", resp.StatusCode)
	}
}
