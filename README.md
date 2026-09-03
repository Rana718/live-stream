# Live Platform — multi-tenant edtech SaaS backend

A production-oriented backend for running **coaching institutes online**: each
institute (tenant) gets a branded portal + mobile app, sells courses / bundles /
subscriptions / instalment fee-plans, delivers live + recorded classes, runs
tests and assignments, answers doubts (AI + instructor), and gets GST-compliant
tax invoices for every rupee that moves.

- **Go 1.25 · Fiber v3 · pgx v5 · sqlc · PostgreSQL 16 · Redis · Kafka · MinIO · Razorpay**
- Modular monolith — ~65 packages under `internal/`, one shared `*pgxpool.Pool`.
- **REST** (`/api/v1`, Swagger) **+ gRPC** (optional, server-reflection enabled).
- Row-level security on every tenant table; money is `bigint` paise, never float.
- Reference client: **`school-web`** (Next.js 15 portal — super-admin / admin /
  instructor / student panels) in the sibling directory.

> Schema-v2 rewrite status and the full design rationale live in
> `~/.claude/plans/majestic-wiggling-dongarra.md`; the production audit trail is
> `docs/PRODUCTION-READINESS.md`.

---

## The platform flow

This is the journey the product models, end to end. Every step is seeded in
`migrations/0133_full_platform_seed_demo.sql` (20 numbered phases — read it top
to bottom and the flow reads itself), and the tables each step writes are named
so you can follow the data.

| # | Phase | What happens | Key tables |
|---|---|---|---|
| 0 | **Platform** | We publish marketing content, define default GST rates, and bill each tenant on a plan. | `tax_rates`, `blog_posts`, `faqs`, `cms_pages`, `platform_subscriptions`, `app_builds` |
| 1 | **Lead → onboarding** | A website enquiry converts; a tenant + its settings + custom domain + owner account are created (self-serve). | `leads`, `tenants`, `tenant_settings`, `tenant_domains`, `users`, `auth_identities`, `tenant_users` |
| 2 | **Team** | Owner adds an admin, instructors, front-desk staff, and parent accounts. | `tenant_users`, `user_profiles`, `course_instructors`, `device_tokens`, `notification_preferences` |
| 3 | **Students sign up** | Phone-OTP signup (auto-`student` role), profile with class/board/exam-goal/guardian, a wallet, a referral code. | `users`, `user_profiles`, `wallets`, `referral_codes` |
| 4 | **Curriculum** | Exam categories (platform) → subjects → chapters → topics (per tenant). | `exam_categories`, `subjects`, `chapters`, `topics` |
| 5 | **Catalog** | Courses (free taster / flagship paid / in-review), sections, batches, a recurring weekly schedule. | `courses`, `course_sections`, `batches`, `class_schedules` |
| 6 | **Content** | Typed content bodies, a video asset with 360/720/1080 renditions, live sessions (one ended → recording + chat), and one `course_lesson` of **every** kind (video / document / link / live_session / quiz / assignment). | `content_videos`, `content_documents`, `content_links`, `video_assets`, `video_renditions`, `live_sessions`, `recordings`, `session_messages`, `qr_check_ins`, `course_lessons` |
| 7 | **Assessment** | A reusable question bank (all 5 kinds) with options; a DPP and a full mock built from it. | `question_bank`, `question_options`, `tests`, `test_sections`, `test_questions` |
| 8 | **Commerce catalogue** | A `product` + active `price` for each sellable thing (course / bundle / plan / fee-plan); a 2-course bundle; a subscription plan; a fee plan; a launch coupon. | `products`, `prices`, `course_bundles`, `bundle_items`, `subscription_plans`, `fee_plans`, `coupons`, `coupon_products` |
| 9 | **Paid course purchase** *(the core money flow)* | `order` (paid, coupon applied) → `order_item` (GST split) → `payment` (captured) + Razorpay-Route split → `entitlement` (`source=purchase`) → `enrollment` → **gapless GST invoice** + line item → `coupon_redemption` → `webhook_event` (the captured event) → `outbox` (course.purchased) → `audit_log`. | `orders`, `order_items`, `payments`, `payment_splits`, `entitlements`, `enrollments`, `invoice_number_series`, `invoices`, `invoice_line_items`, `coupon_redemptions`, `webhook_events`, `outbox`, `audit_logs` |
| 10 | **Refund** | Partial goodwill refund → `refund` → **credit note** (own gapless numbering) → order → `partially_refunded`. Entitlement/enrolment stay. | `refunds`, `credit_note_number_series`, `credit_notes`, `credit_note_line_items` |
| 11 | **Bundle purchase** | One order → **two** entitlements (fan-out) → two enrolments → inter-state invoice (IGST). | `orders`, `order_items`, `payments`, `entitlements`, `enrollments`, `invoices` |
| 12 | **Subscription** | `subscription` (active, trial) + order + payment + `entitlement` (`source=subscription`, expires at period end) + invoice. | `subscriptions`, `orders`, `payments`, `entitlements`, `invoices` |
| 13 | **Fee plan** | `fee_account` + 3 `fee_installments`; instalment 1 paid via its own order + payment. | `fee_accounts`, `fee_installments`, `orders`, `payments` |
| 14 | **Manual grant + gift** | Admin comps a scholarship student (`source=manual_grant`); owner gifts the free course (`source=gift`, redeemed by code). | `entitlements`, `course_gifts` |
| 15 | **Learning activity** | Lesson progress, bookmarks, a graded test attempt with responses, attendance (present/late/absent), doubts (AI + accepted instructor answer), an assignment submission (submitted + graded), a completion certificate. | `content_progress`, `lesson_bookmarks`, `test_attempts`, `test_responses`, `attendance`, `doubts`, `doubt_answers`, `assignments`, `assignment_submissions`, `certificates` |
| 16 | **Engagement** | Course reviews, a forum thread + posts (instructor-highlighted), in-course chat, badges + grants, learning streaks, wishlists. | `course_reviews`, `forum_threads`, `forum_posts`, `course_chat_messages`, `badges`, `badge_grants`, `learning_streaks`, `wishlists` |
| 17 | **Referral + payout** | A student who signed up via a referral link buys → referrer gets a ₹500 wallet credit; an instructor revenue-share payout is queued. | `referral_events`, `wallet_transactions`, `wallets`, `payouts`, `payout_items` |
| 18 | **Communication** | In-app notifications (one read) + per-channel deliveries; tenant + batch announcements; a 2-way WhatsApp thread. | `notifications`, `notification_deliveries`, `announcements`, `messaging_threads`, `messaging_messages` |
| 19 | **Platform ops** | Homepage banner, a background job, an active refresh-token session, a consumed OTP, super-admin audit rows. | `banners`, `jobs`, `refresh_tokens`, `otp_codes`, `audit_logs` |

The seed guarantees **every one of the 114 tables has data** — asserted by
`internal/database/seed_completeness_test.go`.

---

## Core conventions

- **Money is `bigint` minor units (paise) + a `currency` column** — never
  `float`/`numeric`. `internal/money` has the `Money` type and the GST split
  helpers (`SplitGSTInclusive`: CGST/SGST intra-state, IGST inter-state, odd
  paise absorbed into SGST/IGST). Prices are stored **GST-inclusive**.
- **Access is decoupled from payment.** `entitlements` is the single
  access-grant source (`source` = `purchase | subscription | coupon |
  manual_grant | fee_plan | gift | bundle`). `enrollments` is a thin per-course
  progress projection of an entitlement. Content checks read entitlements.
- **Typed product registry.** `products(kind, course_id|bundle_id|plan_id|
  fee_plan_id)` — exactly one ref set (DB CHECK). `prices` holds one active
  `amount_minor` per product.
- **Commerce flow:** `products → prices → orders → order_items → payments →
  entitlements → enrollments → invoices`. Checkout + verify is one transaction;
  the GST invoice is generated **inside** the capture tx (`internal/billing`),
  so per-tenant/per-financial-year invoice numbering is gapless on the happy
  path (`INV/2026-27/000001`). A rollback is GST-acceptable ("cancel via credit
  note").
- **Row-level security on every tenant table.** `internal/database/postgres.go`
  `BeforeAcquire` sets the `app.tenant_id` / `app.user_id` / `app.is_super_admin`
  GUCs on whichever pooled connection serves each query, from the context tags
  `database.WithTenant` / `WithSuperAdmin` / `WithPublicLookup`. The app connects
  as `app_user` (NOSUPERUSER NOBYPASSRLS); the server refuses to boot as a
  superuser. Migration `0110` fails if any tenant table lacks the policies;
  `internal/middleware/rls_matrix_test.go` re-checks + runs a cross-tenant deny
  matrix.
- **Auth: phone-OTP + Google only** (no passwords). Opaque DB-backed refresh
  tokens with family rotation + reuse detection. `role` is resolved from
  `tenant_users` per tenant; `super_admin` is not a `tenant_role` — `issueTokens`
  promotes it when `users.is_platform_super_admin`. `switch-org` re-mints the JWT.

---

## Local setup

```bash
# 1. infra (Postgres, Redis, Kafka, MinIO, nginx-rtmp)
docker compose up -d

# 2. migrations + seeds (idempotent; applies migrations/*.sql in order)
./scripts/migrate.sh              # skips *_seed_demo.sql only when APP_ENV=production

# 3. codegen
make sqlc        # sqlc generate  -> internal/database/db  (gitignored)
make proto       # buf lint + generate -> gen/proto        (gitignored)
make swagger     # swag init -> docs

# 4. run
go run ./cmd/server              # :3000  (+ :50051 if GRPC_PORT set)
```

`make bootstrap` chains steps 1–3. `make dev` runs the server with hot-reload.
The other binaries: `cmd/scheduleworker` (materialises `class_schedules` →
`live_sessions`), `cmd/watermarker`.

The reference frontend:

```bash
cd ../school-web && bun install && bun run dev     # :3001, expects the API on :3000
```

---

## Dev credentials & modes

All logins are phone-OTP. In dev the OTP is **`123456`** (`OTP_DEV_MODE=true`).

| Who | Org code | Phone |
|---|---|---|
| **Platform super-admin** | `PLATFORM` | `+919000000000` |
| Minimal demo tenant admin | `DEMO` | `+919000000001` |
| **Full-showcase tenant** — owner/admin | `VWSTUDY` | `+919000100001` |
| … admin | `VWSTUDY` | `+919000100002` |
| … instructors | `VWSTUDY` | `+919000100003`, `+919000100004` |
| … students | `VWSTUDY` | `+919000100010` … `+919000100015` |

Dev-only env flags (all refused in production by `config.Validate`):

- `OTP_DEV_MODE=true` — OTP delivery is faked, code is `OTP_DEV_CODE` (default `123456`).
- `RAZORPAY_DEV_MODE=true` — the gateway is faked: `CreateOrder` returns a
  synthetic `order_dev…` id, signature checks pass, refunds are faked. **The
  entire checkout → invoice → refund → credit-note flow runs with no Razorpay
  account** (the portal detects `order_dev…` and skips the hosted modal).
- `GRPC_PORT=50051` — also start the gRPC server (off by default).
- `APP_ENV=production` — makes `scripts/migrate.sh` skip every `*_seed_demo.sql`.

---

## API

- **REST** — base `/api/v1`. Interactive docs at `http://localhost:3000/swagger`.
  `/health` (liveness), `/health/deep` (Postgres/Redis/MinIO/Kafka), `/metrics`
  (Prometheus).
- **gRPC** — on `GRPC_PORT` when set. Server reflection is on outside production,
  so `grpcurl -plaintext localhost:50051 list` works. Services are thin adapters
  over the same `internal/*` service layer as REST (no logic duplication);
  `internal/grpcserver`. One versioned package per domain —
  `proto/live/<domain>/v1/*.proto` (+ `live/common/v1` for `PageRequest` /
  `Money`) → `gen/proto/` via `make proto` (committed). Auth = the same bearer
  token in the `authorization` metadata key; `AuthService` login/OTP/refresh are
  unauthenticated; errors use standard gRPC codes (`internal/grpcserver/errors.go`),
  don't parse messages. `make install` installs a pinned `buf`. See
  `proto/README.md` for the add-a-domain recipe.

---

## Testing

```bash
go test ./...                                   # unit tests, no infra needed

# integration tests — need a migrated + seeded DB:
TEST_DATABASE_URL='postgres://app_user:app_user_dev_password@localhost:5432/live_platform?sslmode=disable' \
SCHEMA_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/live_platform?sslmode=disable' \
go test ./...
```

Integration coverage (skipped when the env vars are unset):

- `internal/money` — GST split unit + 5000-case reconstitution property test.
- `internal/middleware` — cross-tenant RLS isolation + a per-table deny matrix
  (102 tables checked for ENABLE+FORCE+policies).
- `internal/billing` — 50-goroutine gapless invoice-numbering test.
- `internal/webhooks` — event-level idempotency (`ON CONFLICT DO NOTHING`).
- `internal/database` — schema-v2 invariants (no float money, every FK indexed,
  no FK cycles) **and `TestFullDemoSeed_EveryTablePopulated`** — every table has
  ≥1 row after seeding.

CI (`.github/workflows/backend.yml`) runs `go build`/`go vet`/`go test -race`,
sqlc-drift, and `buf lint` + `buf generate`-drift on every PR.

---

## Repo layout

```
cmd/
  server/            REST + (optional) gRPC + Kafka consumer
  scheduleworker/    class_schedules -> live_sessions materialiser
  watermarker/       recording watermark worker
internal/            ~65 domain packages (auth, courses, courseorders,
                     billing, subscriptions, fees, tests, attendance,
                     doubts, engagement, notifications, platformadmin, …)
  database/          pool + BeforeAcquire RLS hook, sqlc output (gitignored)
  money/             the Money type + GST helpers
  billing/           GST invoices + credit notes
  grpcserver/        gRPC adapters over the service layer
migrations/          0001…0133 — single NNNN_name.sql files, applied in order
sql/queries/         sqlc query definitions (12 domain files)
proto/               buf v2 module; gen/proto/ is generated (gitignored)
scripts/             migrate.sh, migrate-scratch.sh, setup.sh
docs/                PRODUCTION-READINESS.md, MOBILE.md
```

---

## Infra (docker compose)

`postgres:16`, `redis:7`, `kafka` (KRaft), `minio`, `nginx-rtmp` (RTMP
`:1935`, HLS `:8080`). Overlays: `docker-compose.observability.yml`
(Prometheus + Grafana), `docker-compose.caddy.yml` (automatic TLS incl.
on-demand certs for tenant custom domains).

---

## Architecture decisions

- **Modular monolith** — one deployable, clear package boundaries, one pool.
- **Type-safe data layer** — sqlc generates Go from SQL; `SELECT *` / `RETURNING *`
  are banned so the API shape can't drift silently.
- **RLS is the security boundary**, not application `WHERE` clauses — enforced
  at the DB, asserted by migrations and tests, tests run as the restricted role.
- **Event-driven where it helps** — direct Kafka emit today, `outbox` table +
  idempotent consumers as the migration path to a transactional outbox drain.
- **GST-first billing** — immutable invoices, gapless numbering, credit notes;
  e-invoicing/IRN columns reserved.

## Production considerations

- [x] HTTPS/TLS — `docker/Caddyfile` (Let's Encrypt + on-demand TLS for tenant
      domains); `TLS_CERT_FILE`/`TLS_KEY_FILE` fallback.
- [x] Rate limiting — per-IP (`middleware.RateLimit`) and per-tenant.
- [x] Structured logging — `slog`, JSON.
- [x] Monitoring — `/metrics` + `docker/observability/`.
- [x] CI — `.github/workflows/backend.yml` (lint, sqlc-drift, build + test).
- [x] Tuned pgx pool + `db_pool_connections` gauge.
- [x] Request validation — `go-playground/validator`.
- [x] Graceful shutdown — configurable timeout.
- [x] Deep health checks — `/health`, `/health/deep`.
- [x] RLS coverage — migration `0110` gate + `rls_matrix_test.go`.
- [ ] Secrets management — plain env vars today; manual rotation runbook exists
      (`docs/runbooks/rotate-secrets.md`), no automated store yet.
- [ ] Transactional outbox **drain worker** — table + writer ship; direct Kafka
      emit + idempotent consumers cover it for now.
- [ ] The other ~29 gRPC domain services (CourseService is the template).

## License

MIT
