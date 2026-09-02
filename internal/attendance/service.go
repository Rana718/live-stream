// Package attendance — schema-v2. Attendance rows are keyed on live_sessions
// (session_id, was lecture_id) + optional batch_id, with watched_sec / method
// / geo. Subject-breakdown, monthly report, low-attendance and CSV export
// were dropped in the re-baseline and return empty for now (restored with
// dedicated queries in a later pass).
package attendance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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

func normStatus(s string) db.AttendanceStatus {
	switch s {
	case "present", "absent", "late", "excused":
		return db.AttendanceStatus(s)
	case "viewed":
		return db.AttendanceStatus("present")
	default:
		return db.AttendanceStatus("present")
	}
}

type AutoMarkRequest struct {
	SessionID      *uuid.UUID `json:"session_id"`
	LectureID      *uuid.UUID `json:"lecture_id"` // legacy alias
	BatchID        *uuid.UUID `json:"batch_id"`
	JoinTime       time.Time  `json:"join_time"`
	WatchedSeconds int32      `json:"watched_seconds"`
	GeoLat         *float64   `json:"geo_lat"`
	GeoLng         *float64   `json:"geo_lng"`
}

func (r AutoMarkRequest) session() *uuid.UUID {
	if r.SessionID != nil {
		return r.SessionID
	}
	return r.LectureID
}

func (s *Service) AutoMark(ctx context.Context, tenantID, userID uuid.UUID, req AutoMarkRequest) (db.UpsertAttendanceRow, error) {
	status := "present"
	if req.WatchedSeconds > 0 && req.WatchedSeconds < 120 {
		status = "late"
	}
	sid := req.session()
	if sid == nil {
		return db.UpsertAttendanceRow{}, errors.New("session_id required")
	}
	return s.q.UpsertAttendance(ctx, db.UpsertAttendanceParams{
		TenantID:   utils.UUIDToPg(tenantID),
		UserID:     utils.UUIDToPg(userID),
		SessionID:  utils.UUIDToPg(*sid),
		Status:     normStatus(status),
		BatchID:    utils.UUIDPtrToPg(req.BatchID),
		JoinTime:   tsPtr(req.JoinTime),
		WatchedSec: utils.Int4ToPg(req.WatchedSeconds),
		IsAuto:     utils.BoolToPg(true),
		Method:     utils.TextToPg("auto"),
		GeoLat:     float8Ptr(req.GeoLat),
		GeoLng:     float8Ptr(req.GeoLng),
	})
}

func tsPtr(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

type ManualMarkRequest struct {
	UserID    uuid.UUID `json:"user_id" validate:"required"`
	SessionID uuid.UUID `json:"session_id"`
	LectureID uuid.UUID `json:"lecture_id"`
	Status    string    `json:"status" validate:"required"`
	Notes     string    `json:"notes"`
}

func (s *Service) ManualMark(ctx context.Context, tenantID, markerID uuid.UUID, req ManualMarkRequest) (db.UpsertAttendanceRow, error) {
	sid := req.SessionID
	if sid == uuid.Nil {
		sid = req.LectureID
	}
	return s.q.UpsertAttendance(ctx, db.UpsertAttendanceParams{
		TenantID:  utils.UUIDToPg(tenantID),
		UserID:    utils.UUIDToPg(req.UserID),
		SessionID: utils.UUIDToPg(sid),
		Status:    normStatus(req.Status),
		IsAuto:    utils.BoolToPg(false),
		Method:    utils.TextToPg("manual"),
		MarkedBy:  utils.UUIDToPg(markerID),
		Notes:     utils.TextToPg(req.Notes),
	})
}

type BulkMarkItem struct {
	UserID uuid.UUID `json:"user_id"`
	Status string    `json:"status"`
}

func (s *Service) BulkMark(ctx context.Context, tenantID, markerID, sessionID uuid.UUID, items []BulkMarkItem) (int, error) {
	// One status per call in v2's bulk query; group by status.
	byStatus := map[string][]pgtype.UUID{}
	for _, it := range items {
		byStatus[it.Status] = append(byStatus[it.Status], utils.UUIDToPg(it.UserID))
	}
	n := 0
	for st, ids := range byStatus {
		if err := s.q.BulkMarkAttendance(ctx, db.BulkMarkAttendanceParams{
			TenantID:  utils.UUIDToPg(tenantID),
			SessionID: utils.UUIDToPg(sessionID),
			Status:    normStatus(st),
			MarkedBy:  utils.UUIDToPg(markerID),
			UserIds:   ids,
		}); err != nil {
			return n, err
		}
		n += len(ids)
	}
	return n, nil
}

func (s *Service) ListBySession(ctx context.Context, tenantID, sessionID uuid.UUID) ([]db.ListAttendanceBySessionRow, error) {
	return s.q.ListAttendanceBySession(ctx, db.ListAttendanceBySessionParams{
		TenantID: utils.UUIDToPg(tenantID), SessionID: utils.UUIDToPg(sessionID),
	})
}

func (s *Service) ListMine(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]db.ListMyAttendanceRow, error) {
	return s.q.ListMyAttendance(ctx, db.ListMyAttendanceParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
		Limit: limit, Offset: offset,
	})
}

type Stats struct {
	Total      int64 `json:"total"`
	Present    int64 `json:"present"`
	Absent     int64 `json:"absent"`
	Percentage int   `json:"percentage"`
}

func (s *Service) Stats(ctx context.Context, tenantID, userID uuid.UUID) (Stats, error) {
	row, err := s.q.AttendanceStatsForUser(ctx, db.AttendanceStatsForUserParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
	if err != nil {
		return Stats{}, err
	}
	pct := 0
	if row.Total > 0 {
		pct = int(row.Present * 100 / row.Total)
	}
	return Stats{Total: row.Total, Present: row.Present, Absent: row.Total - row.Present, Percentage: pct}, nil
}

func (s *Service) CreateQRCode(ctx context.Context, tenantID, sessionID, creatorID uuid.UUID, ttl time.Duration) (db.CreateQRCheckInRow, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return db.CreateQRCheckInRow{}, err
	}
	return s.q.CreateQRCheckIn(ctx, db.CreateQRCheckInParams{
		TenantID:  utils.UUIDToPg(tenantID),
		SessionID: utils.UUIDToPg(sessionID),
		Code:      hex.EncodeToString(b),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(ttl), Valid: true},
		CreatedBy: utils.UUIDToPg(creatorID),
	})
}

type QRCheckInRequest struct {
	Code   string   `json:"code" validate:"required"`
	GeoLat *float64 `json:"geo_lat"`
	GeoLng *float64 `json:"geo_lng"`
}

func (s *Service) QRCheckIn(ctx context.Context, tenantID, userID uuid.UUID, req QRCheckInRequest) (db.UpsertAttendanceRow, error) {
	qr, err := s.q.GetQRCheckIn(ctx, req.Code)
	if err != nil {
		return db.UpsertAttendanceRow{}, errors.New("invalid or expired QR code")
	}
	if qr.ExpiresAt.Valid && time.Now().After(qr.ExpiresAt.Time) {
		return db.UpsertAttendanceRow{}, errors.New("QR code expired")
	}
	return s.q.UpsertAttendance(ctx, db.UpsertAttendanceParams{
		TenantID:  utils.UUIDToPg(tenantID),
		UserID:    utils.UUIDToPg(userID),
		SessionID: qr.SessionID,
		Status:    normStatus("present"),
		JoinTime:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
		IsAuto:    utils.BoolToPg(true),
		Method:    utils.TextToPg("qr"),
		GeoLat:    float8Ptr(req.GeoLat),
		GeoLng:    float8Ptr(req.GeoLng),
	})
}
