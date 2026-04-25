package attendance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

func float8Ptr(p *float64) pgtype.Float8 {
	if p == nil {
		return pgtype.Float8{Valid: false}
	}
	return pgtype.Float8{Float64: *p, Valid: true}
}

// Constants for attendance policy
const (
	LateGracePeriodMinutes = 10 // join within this many minutes = Present, after = Late
	MinWatchedForAuto      = 600 // 10 minutes of watch time auto-marks Present for recordings
	MinWatchedForViewed    = 120 // 2 minutes = "Viewed" status (lower than Present)
)

type AutoMarkRequest struct {
	LectureID      uuid.UUID  `json:"lecture_id" validate:"required"`
	BatchID        *uuid.UUID `json:"batch_id"`
	JoinTime       time.Time  `json:"join_time"`
	WatchedSeconds int32      `json:"watched_seconds"`
	GeoLat         *float64   `json:"geo_lat"`
	GeoLng         *float64   `json:"geo_lng"`
}

// AutoMark derives present/late/viewed/absent from join time + watched seconds
// using the lecture's scheduled_at as the reference start time.
func (s *Service) AutoMark(ctx context.Context, userID uuid.UUID, req AutoMarkRequest) (*db.Attendance, error) {
	lec, err := s.q.GetLectureByID(ctx, utils.UUIDToPg(req.LectureID))
	if err != nil {
		return nil, fmt.Errorf("lecture not found: %w", err)
	}

	status := "absent"
	scheduled := utils.TimestampFromPg(lec.ScheduledAt)
	if !req.JoinTime.IsZero() && scheduled != nil {
		grace := scheduled.Add(LateGracePeriodMinutes * time.Minute)
		if req.JoinTime.Before(grace) {
			status = "present"
		} else {
			status = "late"
		}
	} else if req.WatchedSeconds >= MinWatchedForAuto {
		status = "present"
	} else if req.WatchedSeconds >= MinWatchedForViewed {
		status = "viewed"
	}

	_ = lec // reserved for future geo-fencing/batch lookup

	a, err := s.q.UpsertAttendance(ctx, db.UpsertAttendanceParams{
		UserID:         utils.UUIDToPg(userID),
		LectureID:      utils.UUIDToPg(req.LectureID),
		BatchID:        utils.UUIDPtrToPg(req.BatchID),
		Status:         status,
		JoinTime:       utils.TimestampToPg(req.JoinTime),
		LeaveTime:      pgtype.Timestamp{Valid: false},
		WatchedSeconds: utils.Int4ToPg(req.WatchedSeconds),
		IsAuto:         utils.BoolToPg(true),
		GeoLat:         float8Ptr(req.GeoLat),
		GeoLng:         float8Ptr(req.GeoLng),
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

type ManualMarkRequest struct {
	UserID    uuid.UUID `json:"user_id" validate:"required"`
	LectureID uuid.UUID `json:"lecture_id" validate:"required"`
	Status    string    `json:"status" validate:"required,oneof=present absent late excused viewed"`
	Notes     string    `json:"notes"`
}

func (s *Service) ManualMark(ctx context.Context, instructorID uuid.UUID, req ManualMarkRequest) (*db.Attendance, error) {
	a, err := s.q.UpsertAttendance(ctx, db.UpsertAttendanceParams{
		UserID:    utils.UUIDToPg(req.UserID),
		LectureID: utils.UUIDToPg(req.LectureID),
		Status:    req.Status,
		IsAuto:    utils.BoolToPg(false),
		MarkedBy:  utils.UUIDToPg(instructorID),
		Notes:     utils.TextToPg(req.Notes),
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

type BulkMarkItem struct {
	UserID uuid.UUID `json:"user_id" validate:"required"`
	Status string    `json:"status" validate:"required"`
	Notes  string    `json:"notes"`
}

func (s *Service) BulkMark(ctx context.Context, instructorID, lectureID uuid.UUID, items []BulkMarkItem) (int, error) {
	count := 0
	for _, it := range items {
		_, err := s.q.UpsertAttendance(ctx, db.UpsertAttendanceParams{
			UserID:    utils.UUIDToPg(it.UserID),
			LectureID: utils.UUIDToPg(lectureID),
			Status:    it.Status,
			IsAuto:    utils.BoolToPg(false),
			MarkedBy:  utils.UUIDToPg(instructorID),
			Notes:     utils.TextToPg(it.Notes),
		})
		if err == nil {
			count++
		}
	}
	return count, nil
}

func (s *Service) ListByLecture(ctx context.Context, lectureID uuid.UUID) ([]db.ListAttendanceByLectureRow, error) {
	return s.q.ListAttendanceByLecture(ctx, utils.UUIDToPg(lectureID))
}

func (s *Service) ListMine(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.ListMyAttendanceRow, error) {
	return s.q.ListMyAttendance(ctx, db.ListMyAttendanceParams{
		UserID: utils.UUIDToPg(userID),
		Limit:  limit, Offset: offset,
	})
}

func (s *Service) ListByBatch(ctx context.Context, batchID uuid.UUID, limit, offset int32) ([]db.ListAttendanceByBatchRow, error) {
	return s.q.ListAttendanceByBatch(ctx, db.ListAttendanceByBatchParams{
		BatchID: utils.UUIDToPg(batchID),
		Limit:   limit, Offset: offset,
	})
}

type UserAttendanceStats struct {
	TotalClasses int64   `json:"total_classes"`
	Attended     int64   `json:"attended"`
	Percent      float64 `json:"percent"`
}

func (s *Service) UserPercent(ctx context.Context, userID uuid.UUID) (*UserAttendanceStats, error) {
	row, err := s.q.AttendancePercentByUser(ctx, utils.UUIDToPg(userID))
	if err != nil {
		return nil, err
	}
	return &UserAttendanceStats{
		TotalClasses: row.TotalClasses,
		Attended:     row.Attended,
		Percent:      utils.NumericToFloat(row.Percent),
	}, nil
}

type SubjectPercent struct {
	SubjectID    string  `json:"subject_id"`
	TotalClasses int64   `json:"total_classes"`
	Attended     int64   `json:"attended"`
	Percent      float64 `json:"percent"`
}

func (s *Service) UserSubjectBreakdown(ctx context.Context, userID uuid.UUID) ([]SubjectPercent, error) {
	rows, err := s.q.AttendancePercentByUserSubject(ctx, utils.UUIDToPg(userID))
	if err != nil {
		return nil, err
	}
	out := make([]SubjectPercent, 0, len(rows))
	for _, r := range rows {
		out = append(out, SubjectPercent{
			SubjectID:    utils.UUIDFromPg(r.SubjectID),
			TotalClasses: r.TotalClasses,
			Attended:     r.Attended,
			Percent:      utils.NumericToFloat(r.Percent),
		})
	}
	return out, nil
}

type DailyReport struct {
	Day     time.Time `json:"day"`
	Total   int64     `json:"total"`
	Present int64     `json:"present"`
	Late    int64     `json:"late"`
	Absent  int64     `json:"absent"`
}

func (s *Service) MonthlyReport(ctx context.Context, userID uuid.UUID, month time.Time) ([]DailyReport, error) {
	rows, err := s.q.AttendanceMonthlyReport(ctx, db.AttendanceMonthlyReportParams{
		UserID:  utils.UUIDToPg(userID),
		Column2: utils.TimestampToPg(month),
	})
	if err != nil {
		return nil, err
	}
	out := make([]DailyReport, 0, len(rows))
	for _, r := range rows {
		day := time.Time{}
		if r.Day.Valid {
			day = r.Day.Time
		}
		out = append(out, DailyReport{
			Day: day, Total: r.Total, Present: r.Present, Late: r.Late, Absent: r.Absent,
		})
	}
	return out, nil
}

type LowAttendanceRow struct {
	UserID   string  `json:"user_id"`
	Email    string  `json:"email"`
	FullName string  `json:"full_name"`
	Total    int64   `json:"total"`
	Attended int64   `json:"attended"`
	Percent  float64 `json:"percent"`
}

func (s *Service) LowAttendance(ctx context.Context, batchID *uuid.UUID, threshold float64) ([]LowAttendanceRow, error) {
	rows, err := s.q.LowAttendanceUsers(ctx, db.LowAttendanceUsersParams{
		Column1: utils.UUIDPtrToPg(batchID),
		Status:  fmt.Sprintf("%.2f", threshold),
	})
	if err != nil {
		return nil, err
	}
	out := make([]LowAttendanceRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, LowAttendanceRow{
			UserID:   utils.UUIDFromPg(r.UserID),
			Email:    utils.TextFromPg(r.Email),
			FullName: utils.TextFromPg(r.FullName),
			Total:    r.Total,
			Attended: r.Attended,
			Percent:  utils.NumericToFloat(r.Percent),
		})
	}
	return out, nil
}

func (s *Service) ExportBatchRange(ctx context.Context, batchID uuid.UUID, from, to time.Time) ([]db.ExportAttendanceByBatchRow, error) {
	return s.q.ExportAttendanceByBatch(ctx, db.ExportAttendanceByBatchParams{
		BatchID: utils.UUIDToPg(batchID),
		CreatedAt: utils.TimestampToPg(from),
		CreatedAt_2: utils.TimestampToPg(to),
	})
}

// --- QR code attendance ---

func (s *Service) CreateQRCode(ctx context.Context, lectureID, instructorID uuid.UUID, ttl time.Duration) (*db.ClassQrCode, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	code := hex.EncodeToString(b)
	qr, err := s.q.CreateQRCode(ctx, db.CreateQRCodeParams{
		LectureID: utils.UUIDToPg(lectureID),
		Code:      code,
		ExpiresAt: utils.TimestampToPg(time.Now().Add(ttl)),
		CreatedBy: utils.UUIDToPg(instructorID),
	})
	if err != nil {
		return nil, err
	}
	return &qr, nil
}

type QRCheckInRequest struct {
	Code   string   `json:"code" validate:"required"`
	GeoLat *float64 `json:"geo_lat"`
	GeoLng *float64 `json:"geo_lng"`
}

func (s *Service) QRCheckIn(ctx context.Context, userID uuid.UUID, req QRCheckInRequest) (*db.Attendance, error) {
	qr, err := s.q.GetQRCode(ctx, req.Code)
	if err != nil {
		return nil, errors.New("invalid or expired QR code")
	}
	a, err := s.q.UpsertAttendance(ctx, db.UpsertAttendanceParams{
		UserID:    utils.UUIDToPg(userID),
		LectureID: qr.LectureID,
		Status:    "present",
		JoinTime:  utils.TimestampToPg(time.Now()),
		IsAuto:    utils.BoolToPg(true),
		QrCode:    utils.TextToPg(req.Code),
		GeoLat:    float8Ptr(req.GeoLat),
		GeoLng:    float8Ptr(req.GeoLng),
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}
