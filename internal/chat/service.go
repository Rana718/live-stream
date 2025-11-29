package chat

import (
	"context"
	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	queries *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		queries: db.New(pool),
	}
}

func (s *Service) SaveMessage(ctx context.Context, streamID, userID uuid.UUID, message string) (*db.ChatMessage, error) {
	chatMsg, err := s.queries.CreateChatMessage(ctx, db.CreateChatMessageParams{
		StreamID: pgtype.UUID{Bytes: streamID, Valid: true},
		UserID:   pgtype.UUID{Bytes: userID, Valid: true},
		Message:  message,
	})
	if err != nil {
		return nil, err
	}
	return &chatMsg, nil
}

func (s *Service) GetMessagesByStream(ctx context.Context, streamID uuid.UUID, limit, offset int32) ([]db.GetChatMessagesByStreamIDRow, error) {
	return s.queries.GetChatMessagesByStreamID(ctx, db.GetChatMessagesByStreamIDParams{
		StreamID: pgtype.UUID{Bytes: streamID, Valid: true},
		Limit:    limit,
		Offset:   offset,
	})
}

func (s *Service) DeleteMessage(ctx context.Context, messageID uuid.UUID) error {
	return s.queries.DeleteChatMessage(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
}
