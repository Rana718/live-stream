package database

import (
	"context"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
)

type ComponentStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "ok" | "down"
	Error   string `json:"error,omitempty"`
	Latency int64  `json:"latency_ms"`
}

func checkPostgres(ctx context.Context, pool *pgxpool.Pool) ComponentStatus {
	start := time.Now()
	s := ComponentStatus{Name: "postgres"}
	if err := pool.Ping(ctx); err != nil {
		s.Status = "down"
		s.Error = err.Error()
	} else {
		s.Status = "ok"
	}
	s.Latency = time.Since(start).Milliseconds()
	return s
}

func checkRedis(ctx context.Context, rc *redis.Client) ComponentStatus {
	start := time.Now()
	s := ComponentStatus{Name: "redis"}
	if err := rc.Ping(ctx).Err(); err != nil {
		s.Status = "down"
		s.Error = err.Error()
	} else {
		s.Status = "ok"
	}
	s.Latency = time.Since(start).Milliseconds()
	return s
}

func checkMinio(ctx context.Context, mc *minio.Client) ComponentStatus {
	start := time.Now()
	s := ComponentStatus{Name: "minio"}
	_, err := mc.ListBuckets(ctx)
	if err != nil {
		s.Status = "down"
		s.Error = err.Error()
	} else {
		s.Status = "ok"
	}
	s.Latency = time.Since(start).Milliseconds()
	return s
}

func checkKafka(brokers []string, timeout time.Duration) ComponentStatus {
	start := time.Now()
	s := ComponentStatus{Name: "kafka"}
	if len(brokers) == 0 {
		s.Status = "down"
		s.Error = "no brokers configured"
		s.Latency = time.Since(start).Milliseconds()
		return s
	}
	conn, err := net.DialTimeout("tcp", brokers[0], timeout)
	if err != nil {
		s.Status = "down"
		s.Error = err.Error()
	} else {
		_ = conn.Close()
		s.Status = "ok"
	}
	s.Latency = time.Since(start).Milliseconds()
	return s
}

// HealthReport collects component states.
type HealthReport struct {
	Status     string            `json:"status"`
	Components []ComponentStatus `json:"components"`
}

func CollectHealth(ctx context.Context, pool *pgxpool.Pool, rc *redis.Client, mc *minio.Client, kafkaBrokers []string) HealthReport {
	cs := []ComponentStatus{
		checkPostgres(ctx, pool),
		checkRedis(ctx, rc),
		checkMinio(ctx, mc),
		checkKafka(kafkaBrokers, 2*time.Second),
	}
	overall := "ok"
	for _, c := range cs {
		if c.Status != "ok" {
			overall = "degraded"
			break
		}
	}
	return HealthReport{Status: overall, Components: cs}
}
