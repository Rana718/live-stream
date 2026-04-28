package notifications

import (
	"context"
	"fmt"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// BroadcastInput drives the admin /admin/notifications POST. The audience
// switch ("all" | "course" | "user") matches the admin UI's three radios.
//
// We stash the deeplink in resource_id-style metadata; the mobile app
// reads it on tap to navigate. resource_id is a uuid column so we can't
// store the deeplink string there; we put it in title/body for now and
// can promote to a dedicated `deeplink` column later if usage demands.
type BroadcastInput struct {
	Audience string `json:"audience"` // "all" | "course" | "user"
	TargetID string `json:"target_id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Deeplink string `json:"deeplink"`
}

func (s *Service) Broadcast(ctx context.Context, in BroadcastInput) (int, error) {
	if in.Title == "" || in.Body == "" {
		return 0, fmt.Errorf("title and body are required")
	}
	body := in.Body
	if in.Deeplink != "" {
		body = body + "\n\n→ " + in.Deeplink
	}

	switch in.Audience {
	case "user":
		uid, err := uuid.Parse(in.TargetID)
		if err != nil {
			return 0, fmt.Errorf("invalid user id")
		}
		_, err = s.Create(ctx, uid, "broadcast", in.Title, body, "", nil)
		if err != nil {
			return 0, err
		}
		return 1, nil

	case "course":
		cid, err := uuid.Parse(in.TargetID)
		if err != nil {
			return 0, fmt.Errorf("invalid course id")
		}
		// Create the announcement record so it shows in the history UI,
		// then fan out to each enrollee's inbox.
		ann, err := s.q.CreateAnnouncement(ctx, db.CreateAnnouncementParams{
			CourseID:  pgtype.UUID{Bytes: cid, Valid: true},
			Title:     in.Title,
			Body:      body,
			Priority:  utils.TextToPg("normal"),
			CreatedBy: pgtype.UUID{},
		})
		if err != nil {
			return 0, err
		}
		_ = s.q.FanOutToCourseEnrollees(ctx, db.FanOutToCourseEnrolleesParams{
			Type:       "broadcast",
			Title:      in.Title,
			Body:       utils.TextToPg(body),
			ResourceID: ann.ID,
			CourseID:   pgtype.UUID{Bytes: cid, Valid: true},
		})
		return -1, nil // unknown count; fan-out is fire-and-forget

	case "all", "":
		ann, err := s.q.CreateAnnouncement(ctx, db.CreateAnnouncementParams{
			Title:     in.Title,
			Body:      body,
			Priority:  utils.TextToPg("normal"),
			CreatedBy: pgtype.UUID{},
		})
		if err != nil {
			return 0, err
		}
		if err := s.q.FanOutToAllTenantStudents(ctx, db.FanOutToAllTenantStudentsParams{
			Column1:    "broadcast",
			Column2:    in.Title,
			Column3:    body,
			ResourceID: ann.ID,
		}); err != nil {
			return 0, err
		}
		return -1, nil
	}
	return 0, fmt.Errorf("invalid audience: %s", in.Audience)
}

// AdminBroadcast — POST /admin/notifications. Single-shot endpoint the
// admin UI calls regardless of audience.
func (h *Handler) AdminBroadcast(c fiber.Ctx) error {
	var in BroadcastInput
	if err := c.Bind().Body(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	count, err := h.service.Broadcast(c.Context(), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"queued": count})
}
