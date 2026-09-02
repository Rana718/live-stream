// Package notifications — schema-v2. notifications.type → template_key,
// resource_* → entity_*, is_read → read_at (timestamptz). Announcements
// fan out into per-user notification rows via the FanOut* queries.
package notifications

import (
	"context"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

func ntext(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func (s *Service) Create(ctx context.Context, tenantID, userID uuid.UUID, templateKey, title, body, entityType string, entityID *uuid.UUID) (db.CreateNotificationRow, error) {
	return s.q.CreateNotification(ctx, db.CreateNotificationParams{
		TenantID:    utils.UUIDToPg(tenantID),
		UserID:      utils.UUIDToPg(userID),
		TemplateKey: templateKey,
		Title:       title,
		Body:        ntext(body),
		EntityType:  ntext(entityType),
		EntityID:    utils.UUIDPtrToPg(entityID),
	})
}

func (s *Service) ListMine(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]db.ListNotificationsRow, error) {
	return s.q.ListNotifications(ctx, db.ListNotificationsParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
		Limit: limit, Offset: offset,
	})
}

func (s *Service) UnreadCount(ctx context.Context, tenantID, userID uuid.UUID) (int64, error) {
	return s.q.CountUnreadNotifications(ctx, db.CountUnreadNotificationsParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
}

func (s *Service) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	return s.q.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
		ID: utils.UUIDToPg(id), UserID: utils.UUIDToPg(userID),
	})
}

func (s *Service) MarkAllRead(ctx context.Context, tenantID, userID uuid.UUID) error {
	return s.q.MarkAllNotificationsRead(ctx, db.MarkAllNotificationsReadParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
	})
}

func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.q.DeleteNotification(ctx, db.DeleteNotificationParams{
		ID: utils.UUIDToPg(id), UserID: utils.UUIDToPg(userID),
	})
}

// ── announcements ───────────────────────────────────────────────────

type CreateAnnouncementRequest struct {
	BatchID   *uuid.UUID `json:"batch_id"`
	CourseID  *uuid.UUID `json:"course_id"`
	Title     string     `json:"title" validate:"required,min=3"`
	Body      string     `json:"body" validate:"required"`
	Priority  string     `json:"priority"`
	ExpiresAt *time.Time `json:"expires_at"`
	FanOut    bool       `json:"fan_out"`
}

func (s *Service) CreateAnnouncement(ctx context.Context, tenantID, creatorID uuid.UUID, req CreateAnnouncementRequest) (db.CreateAnnouncementRow, error) {
	if req.Priority == "" {
		req.Priority = "normal"
	}
	exp := pgtype.Timestamptz{}
	if req.ExpiresAt != nil {
		exp = pgtype.Timestamptz{Time: *req.ExpiresAt, Valid: true}
	}
	a, err := s.q.CreateAnnouncement(ctx, db.CreateAnnouncementParams{
		TenantID:  utils.UUIDToPg(tenantID),
		CreatedBy: utils.UUIDToPg(creatorID),
		CourseID:  utils.UUIDPtrToPg(req.CourseID),
		BatchID:   utils.UUIDPtrToPg(req.BatchID),
		Title:     req.Title,
		Body:      req.Body,
		Priority:  ntext(req.Priority),
		ExpiresAt: exp,
	})
	if err != nil {
		return db.CreateAnnouncementRow{}, err
	}
	if req.FanOut {
		s.fanOut(ctx, tenantID, req.BatchID, req.CourseID, "announcement", req.Title, req.Body, uuid.UUID(a.ID.Bytes))
	}
	return a, nil
}

func (s *Service) fanOut(ctx context.Context, tenantID uuid.UUID, batchID, courseID *uuid.UUID, tmpl, title, body string, entityID uuid.UUID) {
	eid := utils.UUIDToPg(entityID)
	switch {
	case batchID != nil:
		_ = s.q.FanOutToBatchEnrollees(ctx, db.FanOutToBatchEnrolleesParams{
			TenantID: utils.UUIDToPg(tenantID), TemplateKey: tmpl, Title: title,
			Body: ntext(body), EntityID: eid, BatchID: utils.UUIDToPg(*batchID),
		})
	case courseID != nil:
		_ = s.q.FanOutToCourseEnrollees(ctx, db.FanOutToCourseEnrolleesParams{
			TenantID: utils.UUIDToPg(tenantID), TemplateKey: tmpl, Title: title,
			Body: ntext(body), EntityID: eid, CourseID: utils.UUIDToPg(*courseID),
		})
	default:
		_ = s.q.FanOutToAllTenantStudents(ctx, db.FanOutToAllTenantStudentsParams{
			TenantID: utils.UUIDToPg(tenantID), TemplateKey: tmpl, Title: title,
			Body: ntext(body), EntityID: eid,
		})
	}
}

func (s *Service) ListAnnouncements(ctx context.Context, tenantID uuid.UUID, courseID, batchID *uuid.UUID, limit, offset int32) ([]db.ListAnnouncementsRow, error) {
	return s.q.ListAnnouncements(ctx, db.ListAnnouncementsParams{
		TenantID: utils.UUIDToPg(tenantID),
		CourseID: utils.UUIDPtrToPg(courseID),
		BatchID:  utils.UUIDPtrToPg(batchID),
		Limit:    limit, Offset: offset,
	})
}

func (s *Service) DeleteAnnouncement(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteAnnouncement(ctx, utils.UUIDToPg(id))
}
