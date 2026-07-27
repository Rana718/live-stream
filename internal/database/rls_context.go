package database

import "context"

// rlsContextKey carries the Postgres RLS session GUCs (app.tenant_id,
// app.user_id, app.is_super_admin, app.is_public_lookup) through Go's
// context.Context so NewPostgresPool's BeforeAcquire hook can apply them
// to whichever physical connection ends up serving a given query.
//
// Why context and not a pinned *pgxpool.Conn: every service in this
// codebase holds a db.Queries built once at startup from the shared
// *pgxpool.Pool (db.New(pgPool)) — each query it runs independently
// acquires-and-releases a connection via the pool. There's no single
// "this request's connection" to pin RLS state onto without restructuring
// every service to be constructed per-request instead of at startup. This
// context-propagation + BeforeAcquire approach gets correct per-request
// RLS scoping without that refactor: BeforeAcquire re-applies the GUCs
// fresh on every acquire, using whatever ctx was passed to that specific
// query, regardless of which physical connection the pool hands out.
type rlsContextKey string

const (
	ctxKeyTenantID       rlsContextKey = "app.tenant_id"
	ctxKeyUserID         rlsContextKey = "app.user_id"
	ctxKeyIsSuperAdmin   rlsContextKey = "app.is_super_admin"
	ctxKeyIsPublicLookup rlsContextKey = "app.is_public_lookup"
)

// WithTenant returns a context that scopes every query made with it (via
// a db.Queries backed by a pool built through NewPostgresPool) to the
// given tenant/user for row-level security.
func WithTenant(ctx context.Context, tenantID, userID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyTenantID, tenantID)
	return context.WithValue(ctx, ctxKeyUserID, userID)
}

// WithSuperAdmin returns a context that bypasses tenant-scoped RLS
// policies (via the super_admin_* policies), for platform-staff /
// cross-tenant operations.
func WithSuperAdmin(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyIsSuperAdmin, true)
}

// WithPublicLookup returns a context scoped to the tenant_public_orgcode
// policy, for unauthenticated org-code-to-branding resolution.
func WithPublicLookup(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyIsPublicLookup, true)
}

func rlsValuesFromContext(ctx context.Context) (tenantID, userID string, isSuperAdmin, isPublicLookup bool) {
	tenantID, _ = ctx.Value(ctxKeyTenantID).(string)
	userID, _ = ctx.Value(ctxKeyUserID).(string)
	isSuperAdmin, _ = ctx.Value(ctxKeyIsSuperAdmin).(bool)
	isPublicLookup, _ = ctx.Value(ctxKeyIsPublicLookup).(bool)
	return
}
