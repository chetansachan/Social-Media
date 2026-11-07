# Deployment configuration

This directory holds infrastructure settings that are shared by every service.
Application code is configured entirely through environment variables; nothing
here is required for local development.

## PostgreSQL read/write split

Each database-backed service accepts two URLs:

| Variable | Purpose | Required |
| :--- | :--- | :--- |
| `DATABASE_URL` | Primary. Every write, transaction, and read-your-writes query. | Yes |
| `DATABASE_READ_URL` | Replica. List, detail, and count queries that tolerate lag. | No |

When `DATABASE_READ_URL` is empty the service opens a single pool and reads from
the primary, so a laptop or a single-database deployment needs no extra setup.

```
                  ┌──────────────────────────────┐
                  │          Microservice        │
                  └───┬──────────────────────┬───┘
        DATABASE_URL  │                      │  DATABASE_READ_URL
                      ▼                      ▼
              ┌───────────────┐      ┌───────────────┐
              │ PgBouncer (W) │      │ PgBouncer (R) │
              └───────┬───────┘      └───────┬───────┘
                      ▼                      ▼
              ┌───────────────┐      ┌───────────────┐
              │  PostgreSQL   │─────▶│  PostgreSQL   │
              │   (primary)   │ rep. │ (read replica)│
              └───────────────┘      └───────────────┘
```

### Pillar routing rules

The split is deliberately conservative. Reads that drive a decision or follow a
write stay on the primary, because a replica that is even one second behind can
break a user-visible flow.

| Service | Replica reads | Primary-only reads |
| :--- | :--- | :--- |
| `user-service` | Profile lookup, batch user hydration | Login by email, refresh-token lookup |
| `post-service` | Post detail, author timeline, archived list, likers, liked posts, batch like states | None; every mutation and transaction |
| `follow-service` | Follower/following lists, follower/following counts | Follow-status check (drives the button) |
| `notification-service` | Inbox list, settings read | Unread counts, `GetByID` after aggregation, every device-token read |
| `moderation-service` | Block/mute status and lists, relationship counts | Every block, mute, report insert and resolve |
| `chat-service` | Inbox, thread reads, unread counts, presence | Every message insert, soft delete, and read watermark |

Writes and transactions always use the primary pool. A transaction can never
begin on the replica, so an outbox event can never commit against the wrong
server.

### Pool sizing

| Service | Max connections | Min connections |
| :--- | :--- | :--- |
| `user-service` | `DB_MAX_CONNS` (20) | `DB_MIN_CONNS` (2) |
| `post-service` | `DB_MAX_CONNS` (20) | `DB_MIN_CONNS` (2) |
| `follow-service` | `DB_MAX_CONNS` (20) | `DB_MIN_CONNS` (2) |
| `notification-service` | `DB_MAX_OPEN_CONNS` (25) | `DB_MAX_IDLE_CONNS` (10) |
| `moderation-service` | `DB_MAX_CONNS` (20) | `DB_MIN_CONNS` (2) |
| `chat-service` | `DB_MAX_CONNS` (20) | `DB_MIN_CONNS` (2) |

`notification-service` also reads `DB_CONN_MAX_LIFETIME` (default `5m`), and
`chat-service` adds `PRESENCE_TTL_SECONDS` (60) and `TYPING_TTL_SECONDS` (6).
Keep
application-side pools small when PgBouncer is in front of PostgreSQL: the
pooler is what multiplexes thousands of clients, so a large per-process pool
multiplies across every pod.

### PgBouncer

The files in `pgbouncer/` assume two pooler deployments:

* **writer** – points at the primary and serves `DATABASE_URL`
* **reader** – points at the replica and serves `DATABASE_READ_URL`

Both use `pool_mode = transaction`. pgx is configured with
`DefaultQueryExecMode = pgx.QueryExecModeExec`, which sends unnamed statements;
this is required because a named prepared statement cannot be assumed to survive
a transaction-mode checkout.

Setup:

1. Copy `pgbouncer/pgbouncer.ini` to `/etc/pgbouncer/pgbouncer.ini`.
2. Copy `databases.writer.ini.example` or `databases.reader.ini.example` to
   `/etc/pgbouncer/databases.ini` for each deployment.
3. Copy `userlist.txt.example` to `/etc/pgbouncer/userlist.txt` and replace the
   placeholders with the SCRAM verifiers from `pg_authid` (see the comments in
   that file). Restrict it with `chmod 600` and the `pgbouncer` owner.
4. Point the services at the poolers:

```dotenv
# writer pooler
DATABASE_URL=postgres://socialflow:secret@pgbouncer-writer:6432/socialflow_users?sslmode=disable

# replica pooler (optional)
DATABASE_READ_URL=postgres://socialflow:secret@pgbouncer-reader:6432/socialflow_users?sslmode=disable
```

Use the same database name in both URLs. If you run a single pooler instead of
two, expose distinct database aliases (`socialflow_users_write`,
`socialflow_users_read`) as shown in the writer example, and use those names in
the URLs.

### Verifying the split

* On the writer pooler: `SHOW POOLS;` shows active client connections per
  database. Steady write traffic should appear there and not on the reader.
* On PostgreSQL: `SELECT usename, application_name, client_addr, state FROM
  pg_stat_activity;` — reader connections should come from the reader pooler's
  address.
* Load-test with `DATABASE_READ_URL` unset first to establish a baseline, then
  set it and confirm read QPS leaves the primary.

### Replication lag

There is no cross-service cache invalidation on replication lag. If lag exceeds
the tolerance for a flow, that flow belongs on the primary. To monitor lag:

```sql
SELECT now() - pg_last_xact_replay_timestamp() AS replica_lag;
```

Alert before lag reaches the interval in which a user expects to see their own
write (roughly one second for this product).

## Containers

`docker/Dockerfile.service` builds every service from the repository root. It
copies only the modules the service imports, regenerates `go.work` inside the
build stage, and runs the static binary from `distroless/static` as a non-root
user. The entrypoint defaults to `cmd/server` and falls back to the module root,
which is where `api-gateway` keeps `main`.

```bash
docker build -f deploy/docker/Dockerfile.service \
  --build-arg SERVICE=user-service \
  -t socialflow/user-service:dev .
```

Replace `SERVICE` with `api-gateway`, `post-service`, `follow-service`,
`feed-service`, `notification-service`, `moderation-service`, or `chat-service`.
The build context must be the repository root; `.dockerignore` keeps `.git`,
local binaries, and environment files out of the image.

The images have no shell. Liveness and readiness come from the HTTP admin
server, never from an exec probe, and `SIGTERM` is handled by the service's own
graceful shutdown.

`docker/Dockerfile.migrate` packages `golang-migrate` with every service's
migrations under `/migrations/<service>`.

## Kubernetes

`k8s/` is a kustomize base. It assumes:

* a metrics server for the HPAs,
* an ingress-nginx controller (the Ingress sets nginx annotations),
* an image registry you can push to, and
* secrets created from your secret manager (never from `secret.example.yaml`).

Apply order:

```bash
kubectl apply -f deploy/k8s/namespace.yaml -f deploy/k8s/configmap.yaml

# Create the Secret out of band, for example:
kubectl -n socialflow create secret generic socialflow-secrets \
  --from-literal=jwt-secret="$(openssl rand -hex 32)" \
  --from-literal=users-database-url="postgres://..." \
  --from-literal=posts-database-url="postgres://..." \
  --from-literal=follows-database-url="postgres://..." \
  --from-literal=notifications-database-url="postgres://..." \
  --from-literal=moderation-database-url="postgres://..." \
  --from-literal=chat-database-url="postgres://..."

# Optional read replicas:
#   --from-literal=users-database-read-url="postgres://..."

# Schema first, then the workloads.
kubectl apply -f deploy/k8s/migrate-jobs.yaml
kubectl -n socialflow wait --for=condition=complete job/users-migrate --timeout=120s
kubectl -n socialflow wait --for=condition=complete job/posts-migrate --timeout=120s
kubectl -n socialflow wait --for=condition=complete job/follows-migrate --timeout=120s
kubectl -n socialflow wait --for=condition=complete job/notifications-migrate --timeout=120s
kubectl -n socialflow wait --for=condition=complete job/moderation-migrate --timeout=120s
kubectl -n socialflow wait --for=condition=complete job/chat-migrate --timeout=120s

kubectl apply -k deploy/k8s
kubectl -n socialflow get pods,svc,hpa
```

Jobs are immutable: delete the migration Jobs before applying a new
revision of `migrate-jobs.yaml`.

Update the placeholder host and TLS secret in `k8s/ingress.yaml` first. The
Ingress sends `/ws` to the notification WebSocket service with one-hour proxy
timeouts, and everything else to the GraphQL gateway. Media uploads go from the
client directly to Cloudinary, so the gateway body limit stays at 1 MB.

### Admin ports and probes

Every pod listens on a private admin port that is not exposed through a
Service. Probes and Prometheus scrape that port inside the pod network.

| Service | Admin port | `/readyz` checks |
| :--- | :--- | :--- |
| `api-gateway` | `9090` | Redis (revocation fails closed) |
| `user-service` | `9091` | PostgreSQL primary |
| `post-service` | `9092` | PostgreSQL primary |
| `follow-service` | `9093` | PostgreSQL primary |
| `feed-service` | `9094` | None; Redis is optional and pull-model fallback stays ready |
| `notification-service` | `9095` | PostgreSQL primary |
| `moderation-service` | `9096` | PostgreSQL primary |
| `chat-service` | `9097` | PostgreSQL primary; Redis is optional (chat works, presence does not) |

`/healthz` is always a process-liveness check and does not touch dependencies.
`/metrics` serves a private Prometheus registry; the OpenTelemetry collector or
a ServiceMonitor should scrape it.

### Graceful shutdown

On `SIGTERM` every service:

1. flips `/readyz` to 503 and marks the gRPC health service `NOT_SERVING`,
2. lets the load balancer and kube-proxy remove the endpoint,
3. drains in-flight HTTP requests and gRPC RPCs (15 second bound, then a forced
   stop),
4. closes hijacked WebSocket connections, which `net/http` cannot drain, and
5. closes the database pool, Redis client, and trace exporter.

`terminationGracePeriodSeconds` is 30 for stateless services and 45 for
`notification-service`, leaving room above the 15 second drain bound.

## Observability environment

| Variable | Default | Meaning |
| :--- | :--- | :--- |
| `ADMIN_PORT` | per service (table above) | Private metrics and health listener. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | OTLP/gRPC collector. Empty propagates trace context without exporting. |
| `OTEL_EXPORTER_OTLP_INSECURE` | `true` | Disable TLS to the collector (in-cluster). |
| `OTEL_TRACES_SAMPLE_RATIO` | `0.1` | Head-based sampling ratio. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. Logs are JSON on stdout. |

## Rate limiting

The gateway enforces a Redis sliding window before a GraphQL operation runs.
Budgets are keyed on the root schema field (not the client-supplied operation
name) and on the user id when authenticated, otherwise on the client IP:

| Operation | Default budget |
| :--- | :--- |
| `Login`, `register` | 5 per minute per IP |
| `CreatePost` | 10 per minute per user |
| `getFeed` | 120 per minute per user |
| any other authenticated field | 600 per minute per user |
| any other anonymous field | 120 per minute per IP |

Exceeding a budget returns a GraphQL `RATE_LIMITED` error without executing the
operation. `TRUST_PROXY_HEADERS` must stay `"false"` unless a proxy overwrites
`X-Forwarded-For`, otherwise anonymous clients can spoof their address. If Redis
is unavailable the limiter fails open (requests proceed) while session
revocation keeps failing closed.

## Autoscaling

| Workload | Min | Max | Signal |
| :--- | :--- | :--- | :--- |
| `api-gateway` | 3 | 50 | CPU 70% |
| `notification-service` | 3 | 50 | CPU 70% |
| `moderation-service` | 2 | 10 | CPU 70% |
| `chat-service` | 2 | 20 | CPU 70% |
| `user-service` | 2 | 20 | CPU 70% |
| `post-service` | 2 | 20 | CPU 70% |
| `follow-service` | 2 | 20 | CPU 70% |
| `feed-service` | 2 | 20 | CPU 70% |

The WebSocket gateway waits ten minutes before scaling down, because every
removed pod closes its subscribers' connections. The API gateway scales up on
the next 30 second window, so a traffic spike is absorbed without waiting for a
stabilisation window.

## Load tests

`tests/load` contains k6 scenarios for feed reads, post writes, login, and
WebSocket subscriptions. See `tests/load/README.md` for the environment
variables, thresholds, and how to read the results.
