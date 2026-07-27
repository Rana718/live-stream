package metrics

import (
	"context"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests by method, route, status.",
		},
		[]string{"method", "route", "status"},
	)

	HTTPLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency by method and route.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	InFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Concurrent in-flight HTTP requests.",
		},
	)

	AuthLogins = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_logins_total",
			Help: "Login attempts by outcome (success|failure).",
		},
		[]string{"outcome"},
	)

	KafkaEventsConsumed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_events_consumed_total",
			Help: "Kafka events consumed by topic.",
		},
		[]string{"topic"},
	)

	// PaymentWebhookEvents tracks Razorpay webhook outcomes so an alert can
	// fire on a spike of "rejected"/"error" — see
	// docs/runbooks/oncall-payments-pending.md, the usual first symptom of
	// which is exactly this: webhooks arriving but failing to apply.
	PaymentWebhookEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payment_webhook_events_total",
			Help: "Razorpay webhook events by event type and outcome (applied|rejected|error).",
		},
		[]string{"event", "outcome"},
	)

	// DBPoolConns exposes pgxpool.Stat() so a dashboard/alert can catch
	// connection-pool saturation before it starts queuing requests.
	DBPoolConns = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "db_pool_connections",
			Help: "Postgres connection pool state (state=acquired|idle|max).",
		},
		[]string{"state"},
	)
)

func init() {
	prometheus.MustRegister(HTTPRequests, HTTPLatency, InFlight, AuthLogins,
		KafkaEventsConsumed, PaymentWebhookEvents, DBPoolConns)
}

// ObservePoolStats starts a goroutine that samples the pool's connection
// stats into DBPoolConns every interval, until ctx is canceled. Call once
// at startup with the app's shared *pgxpool.Pool.
func ObservePoolStats(ctx context.Context, pool *pgxpool.Pool, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s := pool.Stat()
				DBPoolConns.WithLabelValues("acquired").Set(float64(s.AcquiredConns()))
				DBPoolConns.WithLabelValues("idle").Set(float64(s.IdleConns()))
				DBPoolConns.WithLabelValues("max").Set(float64(s.MaxConns()))
			}
		}
	}()
}

// Middleware instruments every request with count + latency + in-flight gauge.
// Uses c.Route().Path so high-cardinality path params don't explode the metric.
func Middleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		InFlight.Inc()
		defer InFlight.Dec()

		err := c.Next()

		route := c.Route().Path
		if route == "" {
			route = "unknown"
		}
		status := strconv.Itoa(c.Response().StatusCode())
		HTTPRequests.WithLabelValues(c.Method(), route, status).Inc()
		HTTPLatency.WithLabelValues(c.Method(), route).Observe(time.Since(start).Seconds())
		return err
	}
}

// Handler exposes /metrics for Prometheus to scrape.
func Handler() fiber.Handler {
	h := promhttp.Handler()
	return func(c fiber.Ctx) error {
		reqCtx := c.RequestCtx()
		h.ServeHTTP(newReqWriter(c), reqFromFasthttp(reqCtx))
		return nil
	}
}
