package chat

import (
	"context"
	"time"

	"live-platform/internal/database"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// schema-v2: chat_messages → session_messages (stream_id → session_id,
// message → body, + kind chat|system|pinned). Partitioned by created_at.
type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

type HistoryItem struct {
	ID        string    `json:"id"`
	StreamID  string    `json:"stream_id"`
	SessionID string    `json:"session_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Message   string    `json:"message"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}

// tenantCtx scopes the RLS-forced session_messages / live_sessions tables —
// the WebSocket path has no Fiber TenantContext, so we build it from claims.
func tenantCtx(ctx context.Context, tenantID, userID uuid.UUID) context.Context {
	return database.WithTenant(ctx, tenantID.String(), userID.String())
}

func (s *Service) SaveMessage(ctx context.Context, tenantID, sessionID, userID uuid.UUID, message string) (uuid.UUID, error) {
	row, err := s.q.PostSessionMessage(tenantCtx(ctx, tenantID, userID), db.PostSessionMessageParams{
		TenantID:  utils.UUIDToPg(tenantID),
		SessionID: utils.UUIDToPg(sessionID),
		UserID:    utils.UUIDToPg(userID),
		Body:      message,
		Kind:      pgtype.Text{String: "chat", Valid: true},
	})
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.UUID(row.ID.Bytes), nil
}

func (s *Service) History(ctx context.Context, tenantID, userID, sessionID uuid.UUID, limit, offset int32) ([]HistoryItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.q.ListSessionMessages(tenantCtx(ctx, tenantID, userID), db.ListSessionMessagesParams{
		TenantID:  utils.UUIDToPg(tenantID),
		SessionID: utils.UUIDToPg(sessionID),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]HistoryItem, 0, len(rows))
	// newest-first → reverse to chronological
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		name := utils.TextFromPg(r.FullName)
		if name == "" {
			name = utils.TextFromPg(r.Phone)
		}
		id := utils.UUIDFromPg(r.SessionID)
		out = append(out, HistoryItem{
			ID:        utils.UUIDFromPg(r.ID),
			StreamID:  id,
			SessionID: id,
			UserID:    utils.UUIDFromPg(r.UserID),
			Username:  name,
			Message:   r.Body,
			Kind:      r.Kind,
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// SessionBelongsToTenant guards the WebSocket join.
func (s *Service) SessionBelongsToTenant(ctx context.Context, tenantID, userID, sessionID uuid.UUID) bool {
	_, err := s.q.GetLiveSession(tenantCtx(ctx, tenantID, userID), utils.UUIDToPg(sessionID))
	return err == nil
}

// StreamBelongsToTenant is the legacy name kept for the handler.
func (s *Service) StreamBelongsToTenant(ctx context.Context, tenantID, userID, sessionID uuid.UUID) bool {
	return s.SessionBelongsToTenant(ctx, tenantID, userID, sessionID)
}
