# Test inventory and known gaps

This file records what is tested, what is deliberately not, and what the audit
found. It exists because a coverage number alone does not tell a reader whether
the missing part is a repository that needs a database or a feature nobody
thought to test.

Run everything from the repo root:

```powershell
foreach ($m in @("backend/api-gateway","backend/user-service","backend/post-service",
                 "backend/follow-service","backend/feed-service","backend/notification-service",
                 "backend/moderation-service","backend/chat-service",
                 "backend/platform/observability","backend/proto")) {
    Push-Location $m; go test ./...; Pop-Location
}
```

## What is tested

| Module | Hermetic coverage of hand-written code |
| :--- | :--- |
| api-gateway | 100% of graph package statements, 90% of auth, 97% of ratelimit, 54% of startup wiring |
| api-gateway/graph/dataloader | 76% |
| user-service | 100% models, 78% cache, 74% handler, 72% service |
| post-service | 100% models, 78% cache, 54% handler, 59% service |
| follow-service | 100% models, 77% service, 50% events, 72% handler |
| feed-service | 81% service, 91% handler, 71% events, 65% models |
| notification-service | 72% service, 59% events, 43% dispatchers, 42% handler, 61% models, 22% repository |
| moderation-service | 100% models, 87% service, 47% handler |
| chat-service | 100% models, 90% realtime, 78% presence, 73% service, 70% handler |
| platform/observability | 83% |

The api-gateway graph figure is measured on the statements gqlgen does not
generate: `go test -cover` reports the package as single digits because
`generated.go` dominates the statement count, and every hand-written statement
there is covered.

## What is deliberately not tested

* **Repository packages in every service, and `cmd/server` in every service.**
  The repositories are pgx against PostgreSQL and the entrypoints dial seven gRPC
  backends plus Redis. This environment has neither a database nor a Docker
  daemon, so those suites would be skipped at best and misleading at worst. They
  need a live PostgreSQL or testcontainers.
* **`config` and `database` packages.** Thin env readers and pgx pool plumbing;
  padding the count with them would hide the gap above.
* **`go test -race`.** Requires cgo, and this machine has no C compiler. Plain
  `go test` and `go vet` run clean in all ten modules.

## Defects the audit surfaced

Each is fixed in the commit that explains it:

| Defect | Where |
| :--- | :--- |
| Panic from a nil check on a getter that panics by design, twice | `backend/api-gateway/graphql_tracing.go`, `backend/api-gateway/ratelimit/guard.go` |
| Three user lookups returned `Unknown` instead of `NotFound` | `backend/user-service/internal/handler` |
| A nil UUID event id on the follow outbox envelope | `backend/follow-service/internal/models/outbox.go` |
| A nil settings payload mapped to "everything off" | `backend/notification-service/internal/handler` |
| An empty JWT segment accepted and failing later on parse | `backend/notification-service/internal/handler` |
| Three services missing from the local dev stack | `dev.ps1`, `.air.toml` |
