package metrics

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
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
)

func init() {
	prometheus.MustRegister(HTTPRequests, HTTPLatency, InFlight, AuthLogins, KafkaEventsConsumed)
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
		h.ServeHTTP(reqWriter{c}, reqFromFasthttp(reqCtx))
		return nil
	}
}
