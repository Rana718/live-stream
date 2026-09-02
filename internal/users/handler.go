package users

import (
	"strconv"

	"live-platform/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// GET /users/profile
func (h *Handler) GetProfile(c fiber.Ctx) error {
	role, _ := c.Locals(middleware.LocalRole).(string)
	p, err := h.svc.GetProfile(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), role)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}
	return c.JSON(p)
}

// PUT /users/profile
func (h *Handler) UpdateProfile(c fiber.Ctx) error {
	var req struct {
		FullName  string `json:"full_name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := h.svc.UpdateBasics(c.Context(), middleware.CurrentUserID(c), req.FullName, req.AvatarURL); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return h.GetProfile(c)
}

// POST /users/me/onboarding
func (h *Handler) CompleteOnboarding(c fiber.Ctx) error {
	var req struct {
		FullName      string `json:"full_name"`
		ClassLevel    string `json:"class_level"`
		Board         string `json:"board"`
		ExamGoal      string `json:"exam_goal"`
		GuardianName  string `json:"guardian_name"`
		GuardianPhone string `json:"guardian_phone"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.ClassLevel == "" && req.ExamGoal == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "pick at least one of class_level or exam_goal"})
	}
	if err := h.svc.CompleteOnboarding(c.Context(), middleware.CurrentTenantID(c), middleware.CurrentUserID(c), OnboardingInput{
		FullName: req.FullName, ClassLevel: req.ClassLevel, Board: req.Board,
		ExamGoal: req.ExamGoal, GuardianName: req.GuardianName, GuardianPhone: req.GuardianPhone,
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return h.GetProfile(c)
}

// GET /users  (admin) — tenant member roster
func (h *Handler) ListMembers(c fiber.Ctx) error {
	limit, offset := 25, 0
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
		offset = v
	}
	rows, err := h.svc.ListMembers(c.Context(), middleware.CurrentTenantID(c), c.Query("role"), int32(limit), int32(offset))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id": uuid.UUID(r.UserID.Bytes), "full_name": r.FullName.String,
			"email": r.Email.String, "phone": r.Phone.String,
			"role": string(r.Role), "status": string(r.Status), "joined_at": r.JoinedAt.Time,
		}
	}
	return c.JSON(out)
}
