package tests

import (
	"context"

	"live-platform/internal/database/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SetPublished flips the is_published flag on a test. Used by both
// instructor (their own test) and admin (any test in their tenant).
// RLS in Postgres enforces the cross-tenant guard.
func (s *Service) SetPublished(ctx context.Context, id uuid.UUID, published bool) error {
	_, err := s.q.SetTestPublished(ctx, db.SetTestPublishedParams{
		ID:          pgtype.UUID{Bytes: id, Valid: true},
		IsPublished: pgtype.Bool{Bool: published, Valid: true},
	})
	return err
}

// AdminSetPublished — PATCH /tests/:id/publish.
func (h *Handler) AdminSetPublished(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		IsPublished bool `json:"is_published"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if err := h.service.SetPublished(c.Context(), id, body.IsPublished); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"updated": true})
}
