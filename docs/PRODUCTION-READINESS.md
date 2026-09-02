# Production Readiness & Database Design

Status as of 2026-09-02. This document tracks (a) the critical fixes already
landed and (b) the phased database redesign toward a ClassPlus-grade schema.

---

## Part 1 — Critical fixes landed (Phase 1)

| # | Area | What was wrong | Fix |
|---|------|----------------|-----|
| 1 | **Auth — OTP** | `devModeOTP = true` was a hard-coded constant. Every phone accepted `123456` and no real SMS was sent. A full auth bypass in production. | Gated behind `OTP_DEV_MODE` (default **false**). `config.validate()` refuses to boot `ENV=production` with dev mode on. Real SMS provider now required unless dev mode. Added per-phone send throttle (`OTP_MAX_SENDS_PER_HOUR`, Redis-backed). |
| 2 | **Auth — Google** | `/auth/google` trusted client-supplied `{sub,email}`. Anyone could impersonate any user. | New `internal/auth/google` verifier: validates the ID token signature against Google's certs, checks `iss`, `aud` (against `GOOGLE_CLIENT_IDS`), `exp`, and `email_verified`. Client identity fields are ignored. |
| 3 | **Payments — course/bundle purchase** | Order `amount` was written as `NULL`; mark-paid and enrollment were separate un-transacted statements with the enrollment error discarded (`_, _ =`); concurrent verifies double-enrolled / double-rewarded. | `Verify` (and the bundle sibling) now: persist the real amount, run `SELECT … FOR UPDATE` + mark-paid + enrollment in **one transaction**, fail loudly if enrollment fails, and are idempotent under concurrency. |
| 4 | **Payments — webhook backstop** | Razorpay webhook ran DB writes with **no tenant/RLS context** → every query silently matched zero rows, so the backstop never worked. Also treated `payment.authorized` as success. | Webhook now runs under `WithSuperAdmin` (trusted server-to-server), settles in a transaction, and only acts on `payment.captured`. |
| 5 | **Config validation** | README claimed "startup secret validation"; actual checks were 2 lines. | Production now fails closed on: weak/short/identical JWT secrets, default DB password, `sslmode=disable`, `OTP_DEV_MODE=true`, missing SMS provider, missing Razorpay keys/webhook secret, `minioadmin` credentials, wildcard CORS, missing `GOOGLE_CLIENT_IDS`. |
| 6 | **CORS** | `AllowOrigins: ["*"]` on an authenticated, multi-tenant API. | `CORS_ALLOWED_ORIGINS` allow-list; credentialed CORS only with an explicit list; `*` rejected in production. |
| 7 | **Chat** | `chat.Service` was dead code — messages never persisted, "history" endpoint returned a placeholder. WebSocket had no read limit, no deadlines, no ping/pong, no per-tenant authz. | Service wired in; messages persisted (WS + REST); history endpoint returns real rows; WS hardened (`SetReadLimit`, read/write deadlines, ping/pong, per-socket flood control); join is gated to the caller's tenant. |
| 8 | **AI** | `CLAUDE_MODEL` defaulted to `claude-sonnet-4-6` (not a valid id). | Default is `claude-sonnet-4-5`. |
| 9 | **Schema integrity** | See migration `046_integrity_and_money_fixes.sql`. | Global `users.email` unique constraint dropped (blocked multi-tenant email reuse); `payments.user_id` FK → `RESTRICT` (financial history must outlive the user); `DeleteUser` is now a soft-delete/anonymize; non-negative amount + status-domain CHECKs; revenue/order indexes; `updated_at` auto-touch triggers on every table. |

**Verification:** `go build ./...`, `go vet ./...`, and the full test suite
(incl. DB integration tests with `TEST_DATABASE_URL`) all pass. New tests in
`internal/config/config_test.go`. Server boots clean and the OTP + throttle
paths were exercised end-to-end.

### New environment variables

```env
OTP_DEV_MODE=false            # true only for local dev; refused in production
OTP_DEV_CODE=123456
OTP_TTL_SECONDS=300
OTP_MAX_SENDS_PER_HOUR=5
GOOGLE_CLIENT_IDS=            # comma-separated OAuth client IDs (web,android,ios)
CORS_ALLOWED_ORIGINS=*        # comma-separated; explicit list required in prod
CLAUDE_MODEL=claude-sonnet-4-5
```

---

## Part 2 — Still open (ranked)

### P0 — before real traffic

- **Secrets management** — still plain env vars. Move to Vault / AWS Secrets
  Manager / Doppler. `docs/runbooks/rotate-secrets.md` covers manual rotation.
- **Kafka event delivery is fire-and-forget** — `producer.Emit` errors are
  swallowed; no transactional outbox. If Kafka is down, push notifications,
  analytics roll-ups and referral rewards are silently lost. Add an
  `outbox` table written in the same tx as the state change, drained by a
  worker.
- **RTMP `record-done`** spawns a bare goroutine with `context.Background()`
  — recording uploads are lost on deploy/restart. Move to the outbox/worker.

### P1

- **Access-token revocation** — a deactivated user's / demoted admin's JWT
  stays valid for up to 15 min. Add a `token_version` column on `users`,
  include it in the JWT, bump it on deactivate/role-change/password-reset,
  and check it in `AuthMiddleware` (cache in Redis).
- **Refresh-token reuse detection** — `token_rotation.go` is dead code. Wire
  it: on refresh, blocklist the presented token's hash; if a blocklisted
  token is presented again, revoke the whole session family and alert.
- **`BeforeAcquire` double round-trip** — every query does a `SET_CONFIG`
  round-trip before the real query (2× DB RTT). Acceptable at low volume,
  but measure and consider a per-request pinned connection or a
  `SET LOCAL` inside an explicit tx for hot paths.
- **Test coverage** — handlers, middleware, auth, and the RLS boundary have
  almost no coverage. Target: every money path + every RLS policy.
- **Unchecked `c.Locals(...).(uuid.UUID)`** in ~15 handlers → panic → 500
  instead of a clean 401. Add a small typed helper.

### P2

- Structured logging in `RoleMiddleware` (currently stdlib `log.Printf` on
  every request).
- `fmt.Printf` debug lines in `stream/service.go`.
- Chat `Hub` is in-memory — breaks with >1 replica. Move fan-out to Redis
  pub/sub before horizontal scaling.

---

## Part 3 — Database redesign (phased)

70 tables, retrofitted for multi-tenancy in migrations 027–029. It works,
but has accumulated inconsistencies. None of the below is urgent; do it in
order, each phase behind its own migration + backfill.

### 3.1 Money — single convention

**Problem:** amounts are stored three different ways.

| Table | Column | Unit |
|-------|--------|------|
| `courses` | `price NUMERIC(10,2)` | rupees |
| `payments` | `amount NUMERIC(10,2)` | rupees |
| `course_bundles` | `price_paise INTEGER` | paise |
| `platform_subscriptions` | `amount INTEGER` | paise |

**Target:** all money as `BIGINT` **paise**, column suffix `_paise`, plus a
`currency CHAR(3)` everywhere. Rupees columns become generated views for
backwards-compat during the transition. Rounding bugs from float rupees
disappear.

### 3.2 Payment status — canonical enum

`created / pending / authorized / paid / captured / completed` are used
interchangeably today (CHECK in migration 046 currently accepts all).

**Target:** a Postgres enum `payment_status` = `pending | authorized |
captured | failed | refunded | partially_refunded`. One writer helper.
`captured` is the only "money received" state.

### 3.3 Orders vs payments — split the tables

`payments` currently doubles as an orders table (course_id, bundle via
metadata, subscription_id, student_fee_id, fee_installment_id all hang off
it). This makes every query a special case.

**Target:**

```
orders            (id, tenant_id, user_id, kind, status, amount_paise,
                   currency, provider, provider_order_id UNIQUE, created_at)
order_items       (order_id, item_type, item_id, amount_paise)   -- course | bundle | plan | fee
payments          (id, order_id FK, provider_payment_id, provider_signature,
                   status, amount_paise, captured_at, raw JSONB)
refunds           (id, payment_id FK, provider_refund_id, amount_paise,
                   reason, status, created_at)                    -- promote out of metadata JSON
```

Enrollment / subscription activation / fee settlement all become
"on order captured, fan out `order_items`".

### 3.4 Timestamps — `timestamptz` everywhere

Old tables use `timestamp without time zone`; tenant-era tables use
`timestamptz`. During a DST change or a server-TZ move this silently
corrupts ordering.

**Target:** `ALTER … TYPE timestamptz USING col AT TIME ZONE 'UTC'` on every
column (maintenance window — rewrites each table). Standardise on `now()`
default and the `updated_at` trigger (added in 046).

### 3.5 Roles — one source of truth

`users.role` **and** `tenant_users.role` both exist; the JWT is minted from
`users.role` while `tenant_users` is described as authoritative.

**Target:** `tenant_users(tenant_id, user_id, role)` is the only role store.
`users.role` is dropped. Login resolves the role from `tenant_users` for the
chosen tenant. A user can be `student` in org A and `instructor` in org B
without a second `users` row.

### 3.6 Soft-delete + retention

Only `users` has soft-delete (added in 046). `ON DELETE CASCADE` from
`users`/`courses`/`tenants` will happily wipe attendance, submissions,
test attempts and payment history.

**Target:** `deleted_at TIMESTAMPTZ` on every business table; a partial
index `WHERE deleted_at IS NULL`; RLS policies extended with
`AND deleted_at IS NULL` for normal reads; CASCADE downgraded to RESTRICT
on anything holding financial or compliance data.

### 3.7 High-volume tables — partition

`chat_messages`, `lecture_views`, `notifications`, `audit_logs`,
`test_answers` grow unbounded.

**Target:** monthly `PARTITION BY RANGE (created_at)`, a `pg_partman` or
cron job to pre-create partitions, and a retention job that detaches +
drops old partitions (chat: 90d, lecture_views: keep aggregates only after
180d, audit_logs: 400d).

### 3.8 Indexing pass

Every `tenant_id`-scoped table should have its hot query paths covered by a
`(tenant_id, …)` composite. Audit with `pg_stat_statements` +
`pg_stat_user_indexes` after a load test; drop unused indexes (there are
several single-column ones shadowed by composites).

### 3.9 Referential niceties

- `enrollments` unique key is `(user_id, course_id)` — add `tenant_id` for
  clarity and to match the RLS predicate.
- `streams.stream_key` should be `CITEXT` or have a format CHECK; it's a
  security token (RTMP ingest).
- FK indexes: Postgres does **not** auto-index FK columns. Several
  `*_id` FKs (e.g. `batches.instructor_id`, `assignment_submissions.graded_by`)
  have no index → slow cascades and joins.

---

## Migration sequence (proposed)

```
046  integrity_and_money_fixes           ✅ applied
047  outbox_table + worker                (P0 — event delivery)
048  users.token_version                  (P1 — token revocation)
049  money_to_paise (phase 1: add cols)   (3.1)
050  money_to_paise (phase 2: backfill)
051  money_to_paise (phase 3: swap)
052  orders_split (new tables, dual-write)
053  orders_split (cut over reads)
054  timestamptz_conversion               (maintenance window)
055  roles_single_source
056  soft_delete_everywhere
057  partition_high_volume_tables
```

Each migration file stays single-statement-safe and idempotent, matching
the existing `scripts/migrate.sh` convention (tracked in
`schema_migrations_applied`).
