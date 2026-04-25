// Package cache is a thin Redis wrapper for hot reads (tenant by org_code,
// course list by tenant). The wrapper centralises:
//   1. JSON marshalling (callers pass typed values, not strings)
//   2. namespacing (every key is prefixed with `school:` so this app
//      doesn't collide with anything else sharing the Redis instance)
//   3. graceful fallback (Redis errors never fail a request — we just
//      miss the cache and let the handler call the DB)
//
// Why not memcached / in-process / etc.: we already run Redis for refresh
// tokens and rate limits, and the cardinality is small (O(tenants) per
// shape). Pulling in a second cache tier isn't worth the operational cost.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "school:"

type Cache struct {
	rc *redis.Client
}

func New(rc *redis.Client) *Cache { return &Cache{rc: rc} }

// Get pulls a JSON-marshalled value into `dst`. Returns (true, nil) on hit,
// (false, nil) on miss. Bubble errors only on actual transport failures
// where the caller might want to retry — typed redis.Nil is silenced.
func (c *Cache) Get(ctx context.Context, key string, dst any) (bool, error) {
	if c == nil || c.rc == nil {
		return false, nil
	}
	raw, err := c.rc.Get(ctx, keyPrefix+key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		// Stale shape? Drop the entry so the next request repopulates.
		_ = c.rc.Del(ctx, keyPrefix+key).Err()
		return false, nil
	}
	return true, nil
}

// Set writes `val` JSON-encoded with a TTL. Errors are non-fatal — the
// caller has already returned a response by the time we miss the write.
func (c *Cache) Set(ctx context.Context, key string, val any, ttl time.Duration) {
	if c == nil || c.rc == nil {
		return
	}
	raw, err := json.Marshal(val)
	if err != nil {
		return
	}
	_ = c.rc.Set(ctx, keyPrefix+key, raw, ttl).Err()
}

// Invalidate drops one or more keys. Used after a write so the next read
// repopulates with fresh data. Pattern-based deletes are rare here so we
// don't bother with SCAN — caller passes the exact keys.
func (c *Cache) Invalidate(ctx context.Context, keys ...string) {
	if c == nil || c.rc == nil || len(keys) == 0 {
		return
	}
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = keyPrefix + k
	}
	_ = c.rc.Del(ctx, prefixed...).Err()
}

// ─── Common key builders ────────────────────────────────────────────────
// Centralising these prevents typos that would otherwise leave stale rows
// alive forever. Every consumer references one of these.

func KeyTenantByOrgCode(code string) string { return "tenant:org:" + code }
func KeyTenantByID(id string) string        { return "tenant:id:" + id }
func KeyCourseList(tenantID string) string  { return "courses:list:" + tenantID }
func KeyCourse(courseID string) string      { return "course:" + courseID }
