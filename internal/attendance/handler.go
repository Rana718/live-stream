package attendance

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/middleware"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func attendanceToMap(a *db.Attendance) fiber.Map {
	return fiber.Map{
		"id":              utils.UUIDFromPg(a.ID),
		"user_id":         utils.UUIDFromPg(a.UserID),
		"lecture_id":      utils.UUIDFromPg(a.LectureID),
		"batch_id":        utils.UUIDFromPg(a.BatchID),
		"status":          a.Status,
		"join_time":       a.JoinTime,
		"leave_time":      a.LeaveTime,
		"watched_seconds": utils.Int4FromPg(a.WatchedSeconds),
		"is_auto":         utils.BoolFromPg(a.IsAuto),
		"marked_by":       utils.UUIDFromPg(a.MarkedBy),
		"notes":           utils.TextFromPg(a.Notes),
		"created_at":      a.CreatedAt,
	}
}

func parsePagination(c fiber.Ctx) (int32, int32) {
	limit := int32(20)
	offset := int32(0)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = int32(o)
	}
	return limit, offset
}

// AutoMark godoc
// @Summary Auto-mark attendance for the current user (called from join or watch events)
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/auto [post]
func (h *Handler) AutoMark(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req AutoMarkRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	a, err := h.service.AutoMark(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(attendanceToMap(a))
}

// ManualMark godoc
// @Summary Instructor manually marks a single student's attendance
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/manual [post]
func (h *Handler) ManualMark(c fiber.Ctx) error {
	instID, _ := c.Locals("userID").(uuid.UUID)
	var req ManualMarkRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	a, err := h.service.ManualMark(c.Context(), instID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(attendanceToMap(a))
}

// BulkMark godoc
// @Summary Bulk-mark attendance for a lecture (instructor)
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/lecture/{id}/bulk [post]
func (h *Handler) BulkMark(c fiber.Ctx) error {
	instID, _ := c.Locals("userID").(uuid.UUID)
	lectureID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid lecture id"})
	}
	var req struct {
		Items []BulkMarkItem `json:"items" validate:"required,min=1,dive"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	n, err := h.service.BulkMark(c.Context(), instID, lectureID, req.Items)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"marked": n})
}

// ListByLecture godoc
// @Summary Get attendance roster for a lecture
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/lecture/{id} [get]
func (h *Handler) ListByLecture(c fiber.Ctx) error {
	lectureID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid lecture id"})
	}
	rows, err := h.service.ListByLecture(c.Context(), lectureID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":              utils.UUIDFromPg(r.ID),
			"user_id":         utils.UUIDFromPg(r.UserID),
			"full_name":       utils.TextFromPg(r.FullName),
			"email":           r.Email,
			"status":          r.Status,
			"join_time":       r.JoinTime,
			"leave_time":      r.LeaveTime,
			"watched_seconds": utils.Int4FromPg(r.WatchedSeconds),
			"is_auto":         utils.BoolFromPg(r.IsAuto),
		}
	}
	return c.JSON(out)
}

// ListMine godoc
// @Summary Current user's attendance history
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/my [get]
func (h *Handler) ListMine(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	limit, offset := parsePagination(c)
	rows, err := h.service.ListMine(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, len(rows))
	for i, r := range rows {
		out[i] = fiber.Map{
			"id":              utils.UUIDFromPg(r.ID),
			"lecture_id":      utils.UUIDFromPg(r.LectureID),
			"lecture_title":   r.LectureTitle,
			"scheduled_at":    r.ScheduledAt,
			"status":          r.Status,
			"join_time":       r.JoinTime,
			"leave_time":      r.LeaveTime,
			"watched_seconds": utils.Int4FromPg(r.WatchedSeconds),
		}
	}
	return c.JSON(out)
}

// GetMyStats godoc
// @Summary Current user's overall attendance percent
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/my/stats [get]
func (h *Handler) GetMyStats(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	stats, err := h.service.UserPercent(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(stats)
}

// GetMySubjectBreakdown godoc
// @Summary Attendance percent broken down by subject
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/my/subjects [get]
func (h *Handler) GetMySubjectBreakdown(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	rows, err := h.service.UserSubjectBreakdown(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// MonthlyReport godoc
// @Summary Monthly attendance report for current user
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/my/monthly [get]
func (h *Handler) MonthlyReport(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	month := time.Now()
	if m := c.Query("month"); m != "" {
		if t, err := time.Parse("2006-01", m); err == nil {
			month = t
		}
	}
	rows, err := h.service.MonthlyReport(c.Context(), userID, month)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// LowAttendance godoc
// @Summary Students below a given attendance threshold (admin/instructor)
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/low [get]
func (h *Handler) LowAttendance(c fiber.Ctx) error {
	threshold := 75.0
	if t, err := strconv.ParseFloat(c.Query("threshold"), 64); err == nil {
		threshold = t
	}
	var batchID *uuid.UUID
	if v := c.Query("batch_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid batch_id"})
		}
		batchID = &id
	}
	rows, err := h.service.LowAttendance(c.Context(), batchID, threshold)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// ExportCSV godoc
// @Summary Export attendance for a batch as CSV (admin/instructor)
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/batch/{id}/export [get]
func (h *Handler) ExportCSV(c fiber.Ctx) error {
	batchID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid batch id"})
	}
	from := time.Now().AddDate(0, -1, 0)
	to := time.Now().AddDate(0, 0, 1)
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			from = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			to = t
		}
	}

	rows, err := h.service.ExportBatchRange(c.Context(), batchID, from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="attendance_%s.csv"`, batchID.String()))

	w := csv.NewWriter(c.Response().BodyWriter())
	defer w.Flush()
	_ = w.Write([]string{"email", "full_name", "lecture_title", "scheduled_at", "status", "join_time", "leave_time", "watched_seconds"})
	for _, r := range rows {
		scheduled := ""
		if r.ScheduledAt.Valid {
			scheduled = r.ScheduledAt.Time.Format(time.RFC3339)
		}
		jt := ""
		if r.JoinTime.Valid {
			jt = r.JoinTime.Time.Format(time.RFC3339)
		}
		lt := ""
		if r.LeaveTime.Valid {
			lt = r.LeaveTime.Time.Format(time.RFC3339)
		}
		_ = w.Write([]string{
			utils.TextFromPg(r.Email),
			utils.TextFromPg(r.FullName),
			r.LectureTitle,
			scheduled,
			r.Status,
			jt,
			lt,
			strconv.Itoa(int(utils.Int4FromPg(r.WatchedSeconds))),
		})
	}
	return nil
}

// CreateQRCode godoc
// @Summary Generate a QR code for in-person attendance (instructor)
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/qr/{lecture_id} [post]
func (h *Handler) CreateQRCode(c fiber.Ctx) error {
	instID, _ := c.Locals("userID").(uuid.UUID)
	lectureID, err := uuid.Parse(c.Params("lecture_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid lecture id"})
	}
	ttlMin := 15
	if v := c.Query("ttl_minutes"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ttlMin = n
		}
	}
	qr, err := h.service.CreateQRCode(c.Context(), lectureID, instID, time.Duration(ttlMin)*time.Minute)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"code":       qr.Code,
		"lecture_id": utils.UUIDFromPg(qr.LectureID),
		"expires_at": qr.ExpiresAt,
	})
}

// QRCheckIn godoc
// @Summary Student scans QR to mark attendance
// @Tags attendance
// @Security BearerAuth
// @Router /attendance/qr/check-in [post]
func (h *Handler) QRCheckIn(c fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uuid.UUID)
	var req QRCheckInRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := middleware.ValidateStruct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	a, err := h.service.QRCheckIn(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(attendanceToMap(a))
}
