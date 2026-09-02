package chat

import (
	"context"
	"time"

	"live-platform/internal/database"
	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	queries *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{queries: db.New(pool)}
}

// HistoryItem is the client-facing shape for a stored chat message.
type HistoryItem struct {
	ID        string    `json:"id"`
	StreamID  string    `json:"stream_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// tenantCtx scopes a query to the given tenant so the RLS-forced
// chat_messages / streams tables are reachable. Chat has no Fiber
// TenantContext on the WebSocket path, so we build it from the JWT claims.
func tenantCtx(ctx context.Context, tenantID, userID uuid.UUID) context.Context {
	return database.WithTenant(ctx, tenantID.String(), userID.String())
}

// SaveMessage persists one chat line. Returns the stored row id.
func (s *Service) SaveMessage(ctx context.Context, tenantID, streamID, userID uuid.UUID, message string) (uuid.UUID, error) {
	row, err := s.queries.CreateChatMessage(tenantCtx(ctx, tenantID, userID), db.CreateChatMessageParams{
		StreamID: pgtype.UUID{Bytes: streamID, Valid: true},
		UserID:   pgtype.UUID{Bytes: userID, Valid: true},
		Message:  message,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.UUID(row.ID.Bytes), nil
}

// History returns the most recent messages for a stream, oldest first.
func (s *Service) History(ctx context.Context, tenantID, userID, streamID uuid.UUID, limit, offset int32) ([]HistoryItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.queries.GetChatMessagesByStreamID(tenantCtx(ctx, tenantID, userID), db.GetChatMessagesByStreamIDParams{
		StreamID: pgtype.UUID{Bytes: streamID, Valid: true},
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]HistoryItem, 0, len(rows))
	// Query returns newest-first; reverse into chronological order.
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		name := r.FullName.String
		if name == "" {
			name = r.PhoneNumber.String
		}
		out = append(out, HistoryItem{
			ID:        uuid.UUID(r.ID.Bytes).String(),
			StreamID:  uuid.UUID(r.StreamID.Bytes).String(),
			UserID:    uuid.UUID(r.UserID.Bytes).String(),
			Username:  name,
			Message:   r.Message,
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// StreamBelongsToTenant guards the WebSocket join: a caller may only join
// the chat of a stream inside their own tenant.
func (s *Service) StreamBelongsToTenant(ctx context.Context, tenantID, userID, streamID uuid.UUID) bool {
	_, err := s.queries.GetStreamByID(tenantCtx(ctx, tenantID, userID),
		pgtype.UUID{Bytes: streamID, Valid: true})
	return err == nil
}
