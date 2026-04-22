package database

import (
	"context"
	"fmt"
	"live-platform/internal/config"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgresPool(cfg *config.DatabaseConfig) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid database config: %w", err)
	}
	if cfg.MaxConns > 0 {
		pc.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		pc.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLifetime > 0 {
		pc.MaxConnLifetime = time.Duration(cfg.MaxConnLifetime) * time.Second
	}
	if cfg.MaxConnIdleTime > 0 {
		pc.MaxConnIdleTime = time.Duration(cfg.MaxConnIdleTime) * time.Second
	}

	// sqlc generates `SearchVector interface{}` for tsvector columns, but pgx v5
	// has no built-in decoder for OID 3614 (tsvector) / 3615 (tsquery), so rows
	// that include those columns fail with "cannot scan unknown type (OID 3614)
	// in text format into *interface{}". Register both as text on every new
	// connection; the API strips search_vector from JSON responses so clients
	// never see it anyway.
	pc.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		tm := conn.TypeMap()
		tm.RegisterType(&pgtype.Type{Name: "tsvector", OID: 3614, Codec: pgtype.TextCodec{}})
		tm.RegisterType(&pgtype.Type{Name: "tsquery", OID: 3615, Codec: pgtype.TextCodec{}})
		return nil
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), pc)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	return pool, nil
}
