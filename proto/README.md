# proto/ — gRPC API definitions

buf v2 module. One versioned package per domain, mirroring `internal/<domain>`:

```
proto/live/common/v1/types.proto   PageRequest, PageResponse, Money, SortOrder
proto/live/<domain>/v1/<domain>.proto   package live.<domain>.v1
```

`make proto` = `buf lint && buf generate` → `gen/proto/live/<domain>/v1/*.pb.go`
(package `<domain>v1`), which **is committed** (the Docker build does not run
buf). CI runs `buf lint`, `buf generate` + `git diff --exit-code`, and
`buf breaking`.

## Conventions

- Service `= <Domain>Service`. Every RPC gets its own `<Rpc>Request` /
  `<Rpc>Response` (buf STANDARD `RPC_REQUEST_RESPONSE_UNIQUE`) — never reuse or
  share a message across RPCs.
- Entity messages are nouns (`Course`, `Order`, `Attempt`).
- Money → `live.common.v1.Money` (`int64 minor`, `string currency`) — never a
  float, mirrors `internal/money.Money`.
- Timestamps → `google.protobuf.Timestamp` (WKT, no buf dep).
- Pagination → embed `live.common.v1.PageRequest page` in `List*Request`,
  `PageResponse page` in `List*Response`. Server clamps `limit` to `[1,100]`.
- `status` / `role` / `kind` fields are plain `string` in v1, carrying the DB
  enum label verbatim. (Native proto enums are a v2 consideration.)
- Breaking change = a new `live.<domain>.v2` package, never an edit in place.

## Adding a domain

1. Write `proto/live/<domain>/v1/<domain>.proto`; `make proto`.
2. `internal/grpcserver/<domain>.go`:
   - `type <Domain>Server struct { <domain>v1.Unimplemented<Domain>ServiceServer; svc *<domain>.Service }`
   - each RPC: `requireTenant(ctx)` → `c.require(<roles from the REST route's
     middleware in cmd/server/main.go>)` → map proto → the internal request
     struct → `validate(&in)` → `s.svc.<Method>(ctx, …)` → `toStatus(err)` on
     error → map `db.*Row` → proto via `internal/utils` pg helpers + `coerce.go`.
3. Add one `RegisterXServiceServer` line to `registerAll` in `register.go`,
   constructing the service exactly as `cmd/server/main.go` does (same builder
   chains, shared clients from `Deps`).
4. Add an adapter test (`<domain>_test.go`) — happy path + one error path, using
   `grpcserver.WithTestIdentity(ctx, tenant, user, role)` and `testPool`.

## Not exposed over gRPC (REST-only, by design)

Binary uploads (`materials.Upload`, `recording` ingest, `bulkimport.Import`),
the social-share PNG (`share.Render`), the offline-download token flow
(`downloads`), and realtime chat / live-stream fan-out (WebSocket). gRPC covers
those domains' list/get/delete/metadata + status control-plane RPCs only.

`internal/tests` still returns `map[string]any` for attempts; the `tests`
adapter transcribes those keys into explicit `Attempt` / `AttemptAnswer`
messages (`internal/grpcserver/tests.go`) — typing the service is a follow-up.
