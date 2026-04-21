package notifications

import (
	"context"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

// Create inserts a notification targeting a single user.
// Callers use this from other services (e.g., tests after grading, attendance low alert).
func (s *Service) Create(ctx context.Context, userID uuid.UUID, notifType, title, body, resourceType string, resourceID *uuid.UUID) (*db.Notification, error) {
	n, err := s.q.CreateNotification(ctx, db.CreateNotificationParams{
		UserID:       utils.UUIDToPg(userID),
		Type:         notifType,
		Title:        title,
		Body:         utils.TextToPg(body),
		ResourceType: utils.TextToPg(resourceType),
		ResourceID:   utils.UUIDPtrToPg(resourceID),
	})
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Service) ListMine(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.Notification, error) {
	return s.q.ListMyNotifications(ctx, db.ListMyNotificationsParams{
		UserID: utils.UUIDToPg(userID),
		Limit:  limit, Offset: offset,
	})
}

func (s *Service) ListMyUnread(ctx context.Context, userID uuid.UUID, limit int32) ([]db.Notification, error) {
	return s.q.ListMyUnreadNotifications(ctx, db.ListMyUnreadNotificationsParams{
		UserID: utils.UUIDToPg(userID), Limit: limit,
	})
}

func (s *Service) UnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.q.CountMyUnread(ctx, utils.UUIDToPg(userID))
}

func (s *Service) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	return s.q.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
		ID: utils.UUIDToPg(id), UserID: utils.UUIDToPg(userID),
	})
}

func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return s.q.MarkAllMyNotificationsRead(ctx, utils.UUIDToPg(userID))
}

func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.q.DeleteNotification(ctx, db.DeleteNotificationParams{
		ID: utils.UUIDToPg(id), UserID: utils.UUIDToPg(userID),
	})
}

// --- Announcements ---

type CreateAnnouncementRequest struct {
	BatchID   *uuid.UUID `json:"batch_id"`
	CourseID  *uuid.UUID `json:"course_id"`
	Title     string     `json:"title" validate:"required,min=3"`
	Body      string     `json:"body" validate:"required"`
	Priority  string     `json:"priority"`
	ExpiresAt *time.Time `json:"expires_at"`
	FanOut    bool       `json:"fan_out"`
}

func (s *Service) CreateAnnouncement(ctx context.Context, creatorID uuid.UUID, req CreateAnnouncementRequest) (*db.Announcement, error) {
	if req.Priority == "" {
		req.Priority = "normal"
	}
	a, err := s.q.CreateAnnouncement(ctx, db.CreateAnnouncementParams{
		BatchID:   utils.UUIDPtrToPg(req.BatchID),
		CourseID:  utils.UUIDPtrToPg(req.CourseID),
		CreatedBy: utils.UUIDToPg(creatorID),
		Title:     req.Title,
		Body:      req.Body,
		Priority:  utils.TextToPg(req.Priority),
		ExpiresAt: utils.TimestampPtrToPg(req.ExpiresAt),
	})
	if err != nil {
		return nil, err
	}
	if req.FanOut {
		if req.BatchID != nil {
			_ = s.q.FanOutToBatchEnrollees(ctx, db.FanOutToBatchEnrolleesParams{
				Column1:      "announcement",
				Title:        req.Title,
				Body:         utils.TextToPg(req.Body),
				Column4:      a.ID,
				BatchID:      utils.UUIDToPg(*req.BatchID),
			})
		} else if req.CourseID != nil {
			_ = s.q.FanOutToCourseEnrollees(ctx, db.FanOutToCourseEnrolleesParams{
				Column1:  "announcement",
				Title:    req.Title,
				Body:     utils.TextToPg(req.Body),
				Column4:  a.ID,
				CourseID: utils.UUIDToPg(*req.CourseID),
			})
		}
	}
	return &a, nil
}

func (s *Service) ListGlobalAnnouncements(ctx context.Context, limit, offset int32) ([]db.Announcement, error) {
	return s.q.ListAnnouncementsGlobal(ctx, db.ListAnnouncementsGlobalParams{Limit: limit, Offset: offset})
}

func (s *Service) ListBatchAnnouncements(ctx context.Context, batchID uuid.UUID, limit, offset int32) ([]db.Announcement, error) {
	return s.q.ListAnnouncementsByBatch(ctx, db.ListAnnouncementsByBatchParams{
		BatchID: utils.UUIDToPg(batchID), Limit: limit, Offset: offset,
	})
}

func (s *Service) ListCourseAnnouncements(ctx context.Context, courseID uuid.UUID, limit, offset int32) ([]db.Announcement, error) {
	return s.q.ListAnnouncementsByCourse(ctx, db.ListAnnouncementsByCourseParams{
		CourseID: utils.UUIDToPg(courseID), Limit: limit, Offset: offset,
	})
}

func (s *Service) DeleteAnnouncement(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteAnnouncement(ctx, utils.UUIDToPg(id))
}
