package webhooks

import (
	"context"
	"errors"
	"os"
	"testing"

	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The RecordWebhookEvent query is the event-level idempotency gate: a second
// INSERT of the same (gateway, event_id) hits ON CONFLICT DO NOTHING and the
// :one query returns pgx.ErrNoRows. That's what the handler keys off to ack
// a Razorpay retry without re-applying it.
func TestWebhookEvent_SecondInsertIsNoRows(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()

	q := db.New(pool)
	eventID := "payment.captured:pay_" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM webhook_events WHERE gateway='razorpay' AND event_id=$1", eventID)
	})

	p := db.RecordWebhookEventParams{
		Gateway: "razorpay", EventID: eventID, EventType: "payment.captured",
		Payload: []byte(`{"event":"payment.captured"}`), SignatureOk: true,
	}

	if _, err := q.RecordWebhookEvent(ctx, p); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err = q.RecordWebhookEvent(ctx, p)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second insert: want pgx.ErrNoRows, got %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM webhook_events WHERE gateway='razorpay' AND event_id=$1", eventID,
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("want exactly 1 webhook_events row, got %d", n)
	}
}
