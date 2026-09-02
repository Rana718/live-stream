package attendance

import (
	"strconv"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func tval(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func upsertView(a db.UpsertAttendanceRow) fiber.Map {
	return fiber.Map{
		"id":              utils.UUIDFromPg(a.ID),
		"user_id":         utils.UUIDFromPg(a.UserID),
		"session_id":      utils.UUIDFromPg(a.SessionID),
		"lecture_id":      utils.UUIDFromPg(a.SessionID), // legacy alias
		"status":          string(a.Status),
		"watched_seconds": a.WatchedSec,
		"method":          a.Method,
	}
}

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit, offset := int32(50), int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

func (h *Handler) tenant(c fiber.Ctx) uuid.UUID { return middleware.CurrentTenantID(c) }
func (h *Handler) user(c fiber.Ctx) uuid.UUID   { return middleware.CurrentUserID(c) }

// AutoMark — POST /attendance/auto
func (h *Handler) AutoMark(c fiber.Ctx) error {
	var req AutoMarkRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	a, err := h.service.AutoMark(c.Context(), h.tenant(c), h.user(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(upsertView(a))
}

// ManualMark — POST /attendance/manual  (instructor/admin)
func (h *Handler) ManualMark(c fiber.Ctx) error {
	var req ManualMarkRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	a, err := h.service.ManualMark(c.Context(), h.tenant(c), h.user(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(upsertView(a))
}

// BulkMark — POST /attendance/lecture/:id/bulk  (instructor/admin)
func (h *Handler) BulkMark(c fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session id"})
	}
	var req struct {
		Items []BulkMarkItem `json:"items"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	n, err := h.service.BulkMark(c.Context(), h.tenant(c), h.user(c), sessionID, req.Items)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"marked": n})
}

// ListByLecture — GET /attendance/lecture/:id  (roster; :id = session id)
func (h *Handler) ListByLecture(c fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session id"})
	}
	rows, err := h.service.ListBySession(c.Context(), h.tenant(c), sessionID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"user_id":         utils.UUIDFromPg(r.UserID),
			"full_name":       utils.TextFromPg(r.FullName),
			"phone":           utils.TextFromPg(r.Phone),
			"status":          string(r.Status),
			"join_time":       tval(r.JoinTime),
			"leave_time":      tval(r.LeaveTime),
			"watched_seconds": r.WatchedSec,
			"method":          r.Method,
		}
	}
	return c.JSON(out)
}

// ListMine — GET /attendance/my
func (h *Handler) ListMine(c fiber.Ctx) error {
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMine(c.Context(), h.tenant(c), h.user(c), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"session_id":      utils.UUIDFromPg(r.SessionID),
			"lecture_id":      utils.UUIDFromPg(r.SessionID),
			"lecture_title":   r.Title,
			"scheduled_at":    tval(r.ScheduledStart),
			"status":          string(r.Status),
			"join_time":       tval(r.JoinTime),
			"watched_seconds": r.WatchedSec,
		}
	}
	return c.JSON(out)
}

// GetMyStats — GET /attendance/my/stats
func (h *Handler) GetMyStats(c fiber.Ctx) error {
	stats, err := h.service.Stats(c.Context(), h.tenant(c), h.user(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"total": stats.Total, "present": stats.Present, "absent": stats.Absent,
		"percentage": stats.Percentage,
		// legacy keys
		"total_classes": stats.Total, "attended": stats.Present, "percent": stats.Percentage,
	})
}

// GetMySubjectBreakdown — GET /attendance/my/subjects. Dropped in v2 re-baseline.
func (h *Handler) GetMySubjectBreakdown(c fiber.Ctx) error {
	return c.JSON([]fiber.Map{})
}

// MonthlyReport — GET /attendance/my/monthly. Dropped in v2 re-baseline.
func (h *Handler) MonthlyReport(c fiber.Ctx) error {
	return c.JSON([]fiber.Map{})
}

// LowAttendance — GET /attendance/low. Dropped in v2 re-baseline.
func (h *Handler) LowAttendance(c fiber.Ctx) error {
	return c.JSON([]fiber.Map{})
}

// ExportCSV — GET /attendance/batch/:id/export. Dropped in v2 re-baseline.
func (h *Handler) ExportCSV(c fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"error": "CSV export not available in this build"})
}

// CreateQRCode — POST /attendance/qr/:lecture_id  (:lecture_id = session id)
func (h *Handler) CreateQRCode(c fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("lecture_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session id"})
	}
	ttlMin := 15
	if v := c.Query("ttl_minutes"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			ttlMin = n
		}
	}
	qr, err := h.service.CreateQRCode(c.Context(), h.tenant(c), sessionID, h.user(c), time.Duration(ttlMin)*time.Minute)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"code":       qr.Code,
		"session_id": utils.UUIDFromPg(qr.SessionID),
		"lecture_id": utils.UUIDFromPg(qr.SessionID),
		"expires_at": tval(qr.ExpiresAt),
	})
}

// QRCheckIn — POST /attendance/qr/check-in
func (h *Handler) QRCheckIn(c fiber.Ctx) error {
	var req QRCheckInRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	a, err := h.service.QRCheckIn(c.Context(), h.tenant(c), h.user(c), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(upsertView(a))
}
