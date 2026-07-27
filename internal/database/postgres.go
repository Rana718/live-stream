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

	// Every service in this codebase holds a db.Queries built once at
	// startup from this shared pool — there's no single "this request's
	// connection" to pin RLS session GUCs onto (see the long comment on
	// rlsContextKey in rls_context.go for the full reasoning). Instead,
	// BeforeAcquire fires on every single Pool.Query/QueryRow/Exec call
	// (each of which internally does Acquire→run→Release) and re-applies
	// whatever RLS context was attached to *that specific call's* ctx via
	// database.WithTenant/WithSuperAdmin/WithPublicLookup — regardless of
	// which physical connection the pool happens to hand out. A query made
	// with a plain context.Background() (no RLS context attached) clears
	// every GUC to blank, which fails every tenant_isolation_* /
	// super_admin_* policy closed — the safe default is no access, not
	// leftover access from whichever request last used that connection.
	pc.BeforeAcquire = func(ctx context.Context, conn *pgx.Conn) bool {
		tenantID, userID, isSuperAdmin, isPublicLookup := rlsValuesFromContext(ctx)

		setCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := conn.Exec(setCtx,
			"SELECT set_config('app.tenant_id', $1, false), "+
				"set_config('app.user_id', $2, false), "+
				"set_config('app.is_super_admin', $3, false), "+
				"set_config('app.is_public_lookup', $4, false)",
			tenantID, userID, boolGUC(isSuperAdmin), boolGUC(isPublicLookup))
		if err != nil {
			// Reject handing out this connection rather than serving a
			// query with stale/unknown RLS state — pgxpool will close it
			// and try another.
			return false
		}
		return true
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

	if err := refuseIfSuperuser(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func boolGUC(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// refuseIfSuperuser fails startup if DB_USER is a Postgres superuser (or
// otherwise has BYPASSRLS). Every row-level-security policy in this schema
// (migrations/029_rls_policies.sql onward) is the entire mechanism behind
// tenant isolation — and Postgres superusers unconditionally bypass RLS,
// no policy or FORCE ROW LEVEL SECURITY can override that. Connecting as
// `postgres` silently turned every RLS policy in the schema into dead
// weight in every environment this app has ever run in (see
// migrations/043_restricted_app_role.sql for the fix and full story).
// This check exists so that regression can never happen silently again —
// fail loud at startup instead of fail open at query time.
func refuseIfSuperuser(ctx context.Context, pool *pgxpool.Pool) error {
	var isSuper, bypassRLS bool
	err := pool.QueryRow(ctx,
		"SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user",
	).Scan(&isSuper, &bypassRLS)
	if err != nil {
		return fmt.Errorf("checking DB role privileges: %w", err)
	}
	if isSuper || bypassRLS {
		return fmt.Errorf(
			"DB_USER connects as a Postgres superuser or BYPASSRLS role — " +
				"this silently disables every row-level-security tenant-isolation " +
				"policy. Use the restricted app_user role instead (see " +
				"migrations/043_restricted_app_role.sql); keep the superuser " +
				"account for migrations/backups only",
		)
	}
	return nil
}
