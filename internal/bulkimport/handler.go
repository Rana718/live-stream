package bulkimport

import (
	"bytes"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Import — POST /api/v1/admin/users/bulk-import
//
// Accepts multipart form-data (`file` field) OR a raw text/csv body —
// most spreadsheet-export UIs ship as multipart, but cURL / Postman
// users prefer raw, so we tolerate both.
//
//	@Summary  Bulk import students/instructors from CSV
//	@Tags     bulkimport
//	@Security BearerAuth
//	@Accept   text/csv,multipart/form-data
//	@Produce  json
//	@Success  200 {object} Result
//	@Router   /admin/users/bulk-import [post]
func (h *Handler) Import(c fiber.Ctx) error {
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)

	var body []byte
	// Prefer multipart `file` if present — that's what the admin UI uses.
	if fh, err := c.FormFile("file"); err == nil && fh != nil {
		f, err := fh.Open()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "could not open file"})
		}
		defer f.Close()
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(f); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "could not read file"})
		}
		body = buf.Bytes()
	} else {
		body = c.Body()
	}
	if len(body) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "empty body"})
	}

	res, err := h.svc.Import(c.Context(), tenantID, bytes.NewReader(body))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}
