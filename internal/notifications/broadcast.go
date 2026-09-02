package notifications

import (
	"context"
	"fmt"

	"live-platform/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// BroadcastInput drives POST /admin/notifications. Audience: "all"|"course"|"user".
type BroadcastInput struct {
	Audience string `json:"audience"`
	TargetID string `json:"target_id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Deeplink string `json:"deeplink"`
}

func (s *Service) Broadcast(ctx context.Context, tenantID uuid.UUID, in BroadcastInput) (int, error) {
	if in.Title == "" || in.Body == "" {
		return 0, fmt.Errorf("title and body are required")
	}
	body := in.Body
	if in.Deeplink != "" {
		body += "\n\n→ " + in.Deeplink
	}

	switch in.Audience {
	case "user":
		uid, err := uuid.Parse(in.TargetID)
		if err != nil {
			return 0, fmt.Errorf("invalid user id")
		}
		if _, err := s.Create(ctx, tenantID, uid, "broadcast", in.Title, body, "", nil); err != nil {
			return 0, err
		}
		return 1, nil

	case "course":
		cid, err := uuid.Parse(in.TargetID)
		if err != nil {
			return 0, fmt.Errorf("invalid course id")
		}
		ann, err := s.CreateAnnouncement(ctx, tenantID, uuid.Nil, CreateAnnouncementRequest{
			CourseID: &cid, Title: in.Title, Body: body,
		})
		if err != nil {
			return 0, err
		}
		s.fanOut(ctx, tenantID, nil, &cid, "broadcast", in.Title, body, uuid.UUID(ann.ID.Bytes))
		return -1, nil

	case "all", "":
		ann, err := s.CreateAnnouncement(ctx, tenantID, uuid.Nil, CreateAnnouncementRequest{
			Title: in.Title, Body: body,
		})
		if err != nil {
			return 0, err
		}
		s.fanOut(ctx, tenantID, nil, nil, "broadcast", in.Title, body, uuid.UUID(ann.ID.Bytes))
		return -1, nil
	}
	return 0, fmt.Errorf("invalid audience: %s", in.Audience)
}

// AdminBroadcast — POST /admin/notifications
func (h *Handler) AdminBroadcast(c fiber.Ctx) error {
	var in BroadcastInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	count, err := h.service.Broadcast(c.Context(), middleware.CurrentTenantID(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"queued": count})
}
