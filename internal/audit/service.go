package audit

import (
	"context"
	"encoding/json"
	"net/netip"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

// Write records a single audit entry. Errors are swallowed by callers that
// don't want to fail the request on log failure.
//
// tenantID is denormalized onto the row so admin queries can filter per-tenant
// without a join through users.
func (s *Service) Write(ctx context.Context, tenantID, actorID uuid.UUID, actorRole, action, resourceType string,
	resourceID *uuid.UUID, ip string, userAgent string, metadata map[string]any) error {

	var addr *netip.Addr
	if ip != "" {
		if parsed, err := netip.ParseAddr(ip); err == nil {
			addr = &parsed
		}
	}
	metaJSON := []byte("{}")
	if len(metadata) > 0 {
		if b, err := json.Marshal(metadata); err == nil {
			metaJSON = b
		}
	}
	_, err := s.q.WriteAuditLog(ctx, db.WriteAuditLogParams{
		TenantID:     utils.UUIDToPg(tenantID),
		ActorID:      utils.UUIDToPg(actorID),
		ActorRole:    utils.TextToPg(actorRole),
		Action:       action,
		ResourceType: utils.TextToPg(resourceType),
		ResourceID:   utils.UUIDPtrToPg(resourceID),
		Ip:           addr,
		UserAgent:    utils.TextToPg(userAgent),
		Metadata:     metaJSON,
	})
	return err
}

// ListForTenant returns audit rows scoped to a single tenant. Use this for
// the per-tenant admin dashboard. The tenant_id WHERE clause runs ahead of
// the RLS policy on `audit_logs` for an extra defence in depth.
func (s *Service) ListForTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.AuditLog, error) {
	return s.q.ListAuditLogsForTenant(ctx, db.ListAuditLogsForTenantParams{
		TenantID: utils.UUIDToPg(tenantID),
		Limit:    limit,
		Offset:   offset,
	})
}

func (s *Service) List(ctx context.Context, limit, offset int32) ([]db.AuditLog, error) {
	return s.q.ListAuditLogs(ctx, db.ListAuditLogsParams{Limit: limit, Offset: offset})
}

func (s *Service) ListForActor(ctx context.Context, actorID uuid.UUID, limit, offset int32) ([]db.AuditLog, error) {
	return s.q.ListAuditLogsForActor(ctx, db.ListAuditLogsForActorParams{
		ActorID: utils.UUIDToPg(actorID), Limit: limit, Offset: offset,
	})
}
