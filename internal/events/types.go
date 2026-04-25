package events

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Event is the wire format for everything that flows through our single
// Kafka topic. The Type discriminator lets the consumer route to the right
// handler without fishing inside Payload first; TenantID is denormalized
// onto every event so consumers can fan-out per tenant without an extra
// lookup. Payload is opaque JSON the consumer decodes based on Type.
type Event struct {
	ID        uuid.UUID         `json:"id"`
	Type      string            `json:"type"`
	TenantID  uuid.UUID         `json:"tenant_id,omitempty"`
	UserID    uuid.UUID         `json:"user_id,omitempty"`
	Timestamp time.Time         `json:"ts"`
	Payload   map[string]any    `json:"payload,omitempty"`
}

// Event types. Add new ones here so producers + consumers stay in sync.
const (
	TypeUserSignedUp      = "user.signed_up"
	TypeCoursePurchased   = "course.purchased"
	TypeLiveEnded         = "live.ended"
	TypePaymentSucceeded  = "payment.succeeded"
	TypeTenantCreated     = "tenant.created"
	TypeNotificationCreated = "notification.created"
)

// Emit is a convenience wrapper that builds a properly-shaped Event and
// publishes it. Callers should treat publish failures as best-effort —
// bubbling them up would couple the synchronous request path to Kafka
// availability, which we don't want.
func (p *Producer) Emit(ctx context.Context, eventType string, tenantID, userID uuid.UUID, payload map[string]any) {
	if p == nil {
		return
	}
	ev := Event{
		ID:        uuid.New(),
		Type:      eventType,
		TenantID:  tenantID,
		UserID:    userID,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
	// Use tenant_id as the partition key so all events for one tenant
	// land in-order on the same partition (helps consumers that want to
	// process per-tenant state without locking).
	key := tenantID.String()
	if key == uuid.Nil.String() {
		key = ev.ID.String()
	}
	_ = p.PublishEvent(ctx, key, ev)
}
