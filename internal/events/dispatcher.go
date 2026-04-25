package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
)

// Handler is a single subscription. Implementations must be idempotent:
// Kafka redelivers on consumer-group rebalance and we don't want side-effects
// firing twice.
type Handler func(ctx context.Context, ev Event) error

// Dispatcher routes incoming events to all registered handlers for the
// matching Type. Multiple handlers can subscribe to the same Type; each
// runs in its own goroutine so a slow handler doesn't block the consumer.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	log      *slog.Logger
}

func NewDispatcher(log *slog.Logger) *Dispatcher {
	return &Dispatcher{handlers: make(map[string][]Handler), log: log}
}

func (d *Dispatcher) On(eventType string, h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[eventType] = append(d.handlers[eventType], h)
}

// Dispatch parses the raw Kafka payload + invokes every handler for that
// event type. We don't return handler errors to the consumer loop — the
// loop should keep going regardless so one bad message doesn't stall the
// partition.
func (d *Dispatcher) Dispatch(ctx context.Context, raw []byte) {
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		d.log.Warn("event decode failed", slog.String("err", err.Error()))
		return
	}
	d.mu.RLock()
	hs := d.handlers[ev.Type]
	d.mu.RUnlock()

	if len(hs) == 0 {
		// Unknown event types are common in a long-running platform —
		// log at debug to avoid alarming production.
		d.log.Debug("no handler for event", slog.String("type", ev.Type))
		return
	}

	for _, h := range hs {
		go func(handler Handler) {
			defer func() {
				if r := recover(); r != nil {
					d.log.Error("handler panic",
						slog.String("type", ev.Type),
						slog.Any("recover", r))
				}
			}()
			if err := handler(ctx, ev); err != nil {
				d.log.Warn("handler failed",
					slog.String("type", ev.Type),
					slog.String("err", err.Error()))
			}
		}(h)
	}
}

// RunConsumer is a convenience that pulls messages off the consumer and
// pumps them into Dispatch. Exits when ctx is cancelled.
func (d *Dispatcher) RunConsumer(ctx context.Context, c *Consumer) {
	for {
		if ctx.Err() != nil {
			return
		}
		msg, err := c.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			d.log.Warn("kafka read error", slog.String("err", err.Error()))
			continue
		}
		d.Dispatch(ctx, msg.Value)
	}
}
