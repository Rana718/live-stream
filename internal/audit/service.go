// Package audit — schema-v2. audit_logs is partitioned by created_at with
// actor_user_id / entity_type / entity_id / before / after jsonb.
package audit

import (
	"context"
	"encoding/json"
	"net/netip"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

// Write records one audit entry — matches middleware.AuditRecorder. The
// `metadata` map lands in the `after` jsonb column.
func (s *Service) Write(ctx context.Context, tenantID, actorID uuid.UUID, actorRole, action, entityType string,
	entityID *uuid.UUID, ip, userAgent string, metadata map[string]any) error {

	var addr *netip.Addr
	if ip != "" {
		if p, err := netip.ParseAddr(ip); err == nil {
			addr = &p
		}
	}
	var after []byte
	if len(metadata) > 0 {
		if b, err := json.Marshal(metadata); err == nil {
			after = b
		}
	}
	return s.q.WriteAuditLog(ctx, db.WriteAuditLogParams{
		TenantID:    utils.UUIDToPg(tenantID),
		ActorUserID: utils.UUIDToPg(actorID),
		ActorRole:   utils.TextToPg(actorRole),
		Action:      action,
		EntityType:  utils.TextToPg(entityType),
		EntityID:    utils.UUIDPtrToPg(entityID),
		After:       after,
		Ip:          addr,
		UserAgent:   utils.TextToPg(userAgent),
	})
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.ListAuditLogsRow, error) {
	tid := pgtype.UUID{}
	if tenantID != uuid.Nil {
		tid = utils.UUIDToPg(tenantID)
	}
	return s.q.ListAuditLogs(ctx, db.ListAuditLogsParams{Limit: limit, Offset: offset, TenantID: tid})
}
