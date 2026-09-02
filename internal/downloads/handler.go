package downloads

import (
	"live-platform/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

// CreateVariant godoc
// @Summary Register a transcoded video variant (admin/instructor)
// @Tags downloads
// @Security BearerAuth
// @Router /downloads/variants [post]
func (h *Handler) CreateVariant(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)
	var req CreateVariantRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	v, err := h.service.CreateVariant(c.Context(), tenantID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(v)
}

// ListVariantsForLecture godoc
// @Summary List all video qualities available for a lecture
// @Tags downloads
// @Router /downloads/lectures/{lecture_id}/variants [get]
func (h *Handler) ListVariantsForLecture(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("lecture_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid lecture id"})
	}
	rows, err := h.service.ListVariantsForLecture(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// IssueToken godoc
// @Summary Get a time-limited download token for offline use
// @Tags downloads
// @Security BearerAuth
// @Router /downloads/token [post]
func (h *Handler) IssueToken(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req TokenRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	resp, err := h.service.IssueToken(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

// Fetch godoc
// @Summary Redirect to a presigned URL using a download token
// @Tags downloads
// @Router /downloads/fetch [get]
func (h *Handler) Fetch(c fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token required"})
	}
	url, err := h.service.Resolve(c.Context(), token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Redirect().To(url)
}
