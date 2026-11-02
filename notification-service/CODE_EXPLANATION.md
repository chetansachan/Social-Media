# Notification Service — Full Code Walkthrough

This document explains every package, file, and important function in the
`notification-service` module. It describes how a domain event travels from
Redis Streams into a durable, aggregated inbox row, how that row is pushed over
WebSockets, FCM/APNs, and email, and how the gRPC read API serves it back.

If you only read one section, read **§3 Event-to-inbox pipeline** and **§7 The
service layer** — those contain the core behaviour.

---

## 1. What was built

The service implements the architecture in
[`SYSTEM_DESIGN.md`](./SYSTEM_DESIGN.md):

| Capability | Implementation |
| --- | --- |
| Asynchronous ingestion | Redis Streams consumer group (`events/consumer.go`) |
| Idempotency | Redis `SET NX` dedup keys + event IDs (`services/notification_service.go`) |
| Aggregation | Per-entity roll-up with actor sets (`services/aggregator.go`) |
| Durable inbox | PostgreSQL, monthly range partitions (`repository/notification_repo.go`) |
| Preferences & quiet hours | `user_notification_settings` + `services/policy_service.go` |
| Unread badges | Redis counters with PostgreSQL fallback (`repository/cache_repo.go`) |
| gRPC API | `proto/notification.proto` + `handler/grpc_handler.go` |
| Realtime | WebSocket gateway + Redis Pub/Sub fan-out (`handler/ws_*.go`) |
| Push | Provider-agnostic worker pool; FCM v1 and APNs HTTP/2 (`dispatchers/*`) |
| Email | Log and SMTP providers (`dispatchers/email_dispatcher.go`) |
| Retention | Partition drops + bounded row pruning (`repository/notification_repo.go`) |

The folder layout mirrors `post-service` and `feed-service`:

```
cmd/server            process wiring and graceful shutdown
internal/config       environment parsing
internal/database     PostgreSQL and Redis connections
internal/handler      transport adapters (gRPC, WebSocket)
internal/models       domain types, events, cursors, errors
internal/repository   persistence and external adapters
internal/services     business logic
internal/dispatchers  outbound channel delivery
internal/events       stream consumer and event router
migrations            PostgreSQL schema
proto/                shared protobuf contract (outside the module)
```

The module depends on the shared `github.com/chetansachan/socialflow/proto`
module through a `replace` directive and a `go.work` entry, exactly like the
other services.

---

## 2. High-level architecture

```
                        Redis Streams (social:events)
                                   │
                                   ▼
                    ┌──────────────────────────────┐
                    │   events.Consumer            │
                    │   consumer group + XAutoClaim│
                    └──────────────┬───────────────┘
                                   │ models.Event
                                   ▼
                    ┌──────────────────────────────┐
                    │   events.EventHandler        │
                    │   schema → NotificationRequest│
                    └──────────────┬───────────────┘
                                   │
                                   ▼
                    ┌──────────────────────────────┐
                    │  services.NotificationService│
                    │  policy → dedup → aggregate  │
                    └───┬───────────┬───────────┬──┘
                        │           │           │
              PostgreSQL inbox   Redis      Pub/Sub
                        │        counters       │
                        │           │           ▼
                        │           │      WebSocket gateway
                        │           │
                        │           ├──► Push worker pool → FCM / APNs
                        │           └──► Email worker pool → SMTP
                        │
                        ▼
                 gRPC inbox API ◄──── API Gateway / clients
```

Everything after the consumer is transport-neutral. The `NotificationService`
only depends on small interfaces in `services/ports.go`, so the same logic can
be exercised from tests without Redis, PostgreSQL, or gRPC.

---

## 3. Event-to-inbox pipeline

### 3.1 Producer envelope

The consumer accepts three Redis Stream layouts, documented in
`models/events.go`:

```text
1. {"event": "{\"type\":\"post.liked\",\"payload\":{...}}"}
2. {"type": "post.liked", "payload": "{\"post_id\":\"...\"}"}
3. {"type": "post.liked", "post_id": "...", "actor_id": "..."}
```

`EventFromValues` normalises all three into:

```go
type Event struct {
    Type    EventType       `json:"type"`
    Payload json.RawMessage `json:"payload"`
    EventID string          `json:"event_id,omitempty"`
}
```

`NormalizeEventType` accepts spellings such as `post_liked`,
`like.created`, and `POST_LIKED` and maps them to canonical dotted verbs.
Unknown events are preserved and later rejected as permanent errors.

### 3.2 Consumer loop

`events/consumer.go` starts one goroutine per configured stream. For each
stream it:

1. Creates the consumer group with `XGroupCreateMkStream` (idempotent;
   `BUSYGROUP` is ignored).
2. Reads up to 50 new messages with `XReadGroup` and a 2-second block.
3. Every `ClaimInterval` calls `XAutoClaim` to reclaim messages left pending by
   a crashed consumer.
4. For every message:
   - Decodes the event. Malformed messages are moved to the DLQ and acked;
     they can never succeed.
   - Calls the handler.
   - On success: `XACK`.
   - On `models.ErrPermanentEvent`: DLQ + `XACK`.
   - On transient error: leaves the message pending. The reclaim loop retries it.
   - After `MaxAttempts`: DLQ + `XACK`.
5. Attempts are tracked in `notif:attempts:<stream>:<message-id>` with a
   1-hour TTL, so a poison message cannot retry forever.

The DLQ stream stores the original fields prefixed with `event_`, plus
`source_stream`, `source_id`, `error`, and `failed_at`.

### 3.3 Event router

`events/event_handler.go` is the only package that knows both producer schemas
and the domain request type. Each handler parses IDs, validates required
fields, and builds a `models.NotificationRequest`:

| Event | Recipient | Entity | Notes |
| --- | --- | --- | --- |
| `user.followed` | `following_id` | follower ID | deep link to user |
| `user.unfollowed` | — | — | intentionally a no-op |
| `post.liked` | post author | post ID | aggregatable |
| `post.unliked` | post author | post ID | calls `RevokeAggregatedActor` |
| `post.commented` | post author | comment ID | |
| `comment.replied` | parent comment author | reply comment ID | |
| `user.mentioned` | mentioned user | comment or post | |
| `post.created` | explicit recipient | post ID | creator fan-out is upstream |
| `security.alert` | user | `system` | bypasses preferences |
| `user.welcome` | user | `system` | bypasses preferences |

Invalid UUIDs and missing required fields return
`ErrPermanentEvent`; the consumer DLQs them instead of retrying.

### 3.4 Notification request

```go
type NotificationRequest struct {
    RecipientID uuid.UUID
    ActorID     *uuid.UUID
    Type        NotificationType
    EntityID    string
    EntityType  string
    // rendering hints
    ActorUsername, ActorDisplayName, ActorAvatarURL string
    ActionURL, ContentPreview, TitleOverride, BodyOverride string
    BypassPolicy bool
    EventID      string
}
```

`DedupKey()` produces:

```text
dedup:event:<event_id>                                  when EventID is set
dedup:<recipient>:<actor|system>:<type>:<entity_id>     otherwise
```

The recipient is part of the key because one actor can independently act on the
same entity for different recipients.

### 3.5 Processing sequence

`NotificationService.ProcessEvent` runs these steps:

1. **Self-action guard.** If actor and recipient are the same user, return nil.
2. **Validation.** `NotificationRequest.Validate`.
3. **In-app policy.**
   - System alerts skip policy entirely.
   - Otherwise `PolicyService.CheckInApp` consults the user's settings.
   - A settings-store outage **fails open** for in-app notifications: hiding
     the inbox is worse than briefly honouring a muted category.
4. **Dedup.** Acquire the Redis key with the configured TTL.
   - A duplicate returns nil immediately.
   - If Redis is down, dedup is skipped and processing continues.
   - If processing later fails, the key is released so the broker retry is not
     suppressed for the whole TTL.
5. **Route.**
   - Aggregatable type (`post_like`) + actor + aggregator → `processAggregated`.
   - Everything else → `createNotification`.
6. **Persist.** Insert the inbox row. PostgreSQL assigns `id` and timestamps.
7. **Badge.** `INCR unread:<user>` (or a `COUNT(*)` fallback on Redis failure).
8. **Dispatch.**
   - Publish a realtime frame on `notify:<user>`.
   - Enqueue push after evaluating push policy.
   - Enqueue email for security alerts only.

---

## 4. Data model and migrations

### 4.1 `notifications` (000001)

The table is **range-partitioned by month** on `created_at`:

```sql
CREATE TABLE notifications (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    recipient_id UUID NOT NULL,
    type notification_type NOT NULL,
    entity_id VARCHAR(64) NOT NULL,
    entity_type VARCHAR(32) NOT NULL,
    actor_ids TEXT[] NOT NULL DEFAULT '{}',
    total_actors_count INT NOT NULL DEFAULT 1,
    title VARCHAR(120) NOT NULL,
    content_preview VARCHAR(255),
    action_url VARCHAR(512) NOT NULL,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
```

Why a composite primary key? PostgreSQL requires every unique key on a
partitioned table to include the partition key. The UUID alone is still unique
in practice (generated by `gen_random_uuid()`), but the database cannot enforce
that without `created_at`.

Indexes:

- `(recipient_id, created_at DESC, id DESC) WHERE deleted_at IS NULL` — inbox
  pagination.
- `(recipient_id, created_at DESC) WHERE is_read = FALSE AND deleted_at IS NULL`
  — unread fallback.
- `(recipient_id, entity_id, type, created_at DESC)` — aggregation lookup and
  analytics.

The migration also creates the current month plus two future months so a
deployment can never land in a month with no partition. The application repeats
this idempotently at boot and once per day.

`actor_ids` is `TEXT[]` rather than `UUID[]` so pgx can scan it directly into
`[]string`. The values are always UUID strings, validated before insertion.

### 4.2 `user_notification_settings` (000002)

One row per user, with:

- in-app toggles: likes/comments/follows/mentions;
- push master switch plus per-category toggles (likes default **off**);
- email master switch, weekly digest, security alerts;
- quiet-hours window (`TIME`), timezone, and an `updated_at`.

Missing rows are not an error: `DefaultSettings` reproduces the database
defaults in Go.

### 4.3 `user_device_tokens` (000003)

```sql
id UUID PK, user_id UUID, token VARCHAR(512) UNIQUE,
platform CHECK IN ('ios','android','web'),
device_name, app_version, is_active, last_seen_at, created_at
```

The unique token constraint supports `ON CONFLICT (token)` re-registration:
installing the app again reactivates and re-owns the token. The partial index
`(user_id) WHERE is_active = TRUE` is exactly the push worker's query.

---

## 5. Redis key catalogue

| Key | Type | TTL | Purpose |
| --- | --- | --- | --- |
| `unread:<user>` | string | 7 days | unread badge |
| `dedup:event:<event>` | string | `DEDUP_TTL` | global event idempotency |
| `dedup:<user>:<actor>:<type>:<entity>` | string | `DEDUP_TTL` | short duplicate suppression |
| `agg:<user>:<type>:<entity>` | string | aggregation window | maps entity → open notification ID |
| `agg:actors:<agg-key>` | set | aggregation window | actors already counted |
| `agg:rt:<agg-key>` | string | 30 s | realtime throttle |
| `push:rate:<user>:<bucket>` | counter | push window | per-user push budget |
| `prefs:<user>` | JSON | 24 h | settings cache |
| `notif:attempts:<stream>:<id>` | counter | 1 h | consumer retry budget |

Channels:

| Channel | Payload |
| --- | --- |
| `notify:<user>` | `models.RealtimePayload` JSON |

The Pub/Sub channel makes WebSocket delivery work across gateway pods: any pod
can publish, and only the pod holding the socket is subscribed.

---

## 6. Repository layer

### 6.1 `notification_repo.go`

`NotificationRepository` is the durable inbox:

```go
Create(ctx, *CreateNotificationInput) (*Notification, error)
GetByID(ctx, uuid.UUID) (*Notification, error)
List(ctx, recipientID, filter, cursor, limit) ([]Notification, error)
MarkRead(ctx, recipientID, ids) (int, error)
MarkAllRead(ctx, recipientID) (int, error)
Delete(ctx, recipientID, id) (bool, error)
CountUnread(ctx, recipientID) (int, error)
UpdateAggregate(ctx, id, actorIDs, total, title, preview, reopen) (bool, error)
EnsurePartitions(ctx, monthsAhead) error
PruneBefore(ctx, cutoff) (int64, error)
DropPartitionsBefore(ctx, cutoff) (int, error)
```

**Cursor pagination.** `List` uses keyset pagination, not `OFFSET`:

```sql
WHERE recipient_id = $1 AND deleted_at IS NULL
  [AND type = $n::notification_type]
  [AND (created_at, id) < ($n, $n)]
ORDER BY created_at DESC, id DESC
LIMIT $n
```

The row-value comparison matches the composite index exactly. The service asks
for `pageSize + 1` rows to compute `has_more` without a second query.

**`MarkRead`** parses the incoming strings into `[]uuid.UUID` before binding to
`ANY($2::uuid[])`, which gives pgx an unambiguous type. It counts only rows
that actually flipped from unread to read, because the badge decrement must use
that number.

**`Delete`** soft-deletes and returns `!is_read`, so the caller can decrement
the badge without a second query.

**`UpdateAggregate`** is the only statement that needs the *previous* read
state. It locks the row in a CTE and returns that value:

```sql
WITH previous AS (
    SELECT is_read FROM notifications WHERE id = $1 AND deleted_at IS NULL FOR UPDATE
)
UPDATE notifications AS n
SET actor_ids = $2, total_actors_count = $3, title = $4, content_preview = $5,
    updated_at = NOW() [, is_read = FALSE, read_at = NULL]
FROM previous
WHERE n.id = $1
RETURNING previous.is_read
```

`reopen=true` is used when a new actor joins a roll-up. `reopen=false` is used
when a like is retracted: a retraction must not resurrect an already-read
notification.

**Partitions.**

- `EnsurePartitions` loops from the current UTC month through `monthsAhead` and
  executes `CREATE TABLE IF NOT EXISTS ... PARTITION OF ...`. Identifiers are
  sanitised with `pgx.Identifier{}.Sanitize()`; timestamps are formatted in UTC.
- `DropPartitionsBefore` queries `pg_inherits`/`pg_class`, parses names matching
  `notifications_YYYY_MM`, and drops only partitions whose entire range ends
  before the cutoff. This is O(1) per partition instead of a row-by-row delete.
- `PruneBefore` handles the partially expired month that cannot be dropped as a
  whole, deleting in batches of 10 000 to keep transactions short.

### 6.2 `cache_repo.go`

`CacheRepository` is a thin, typed wrapper over Redis. Every method returns
`models.ErrCacheUnavailable` when the client is nil, which is how the service
knows to degrade.

- **Counters.** `IncrUnread` uses a transaction pipeline (`INCR` + `EXPIRE`).
  `DecrUnread` runs a Lua script so the counter is clamped at zero atomically;
  duplicate mark-read calls and retention pruning can never produce a negative
  badge.
- **Dedup.** `AcquireDedup` is `SET NX` with TTL. `ReleaseDedup` exists so a
  failed ingestion does not wait out the full TTL before retrying.
- **Aggregation.** `Get/Set/ClearAggregation` map an aggregation key to an open
  notification ID. `RegisterAggregationActor` is `SADD`; its boolean result
  prevents double counting once the short dedup TTL has expired. The set gets
  the same TTL as the window.
- **Throttle.** `ShouldThrottle` is `SET NX`; the first caller gets `false`
  (send), later callers within the window get `true` (skip).
- **Push budget.** `AllowPush` uses a fixed-window bucket key and a pipeline
  (`INCR` + `EXPIRE`). It is deliberately simple; the goal is alert-fatigue
  protection, not billing-grade accuracy.

### 6.3 `settings_repo.go`

- `Get` checks `prefs:<user>` first and unmarshals the JSON cache.
- On a miss it reads the row, converting `TIME` columns to `HH:MM` with
  `to_char`.
- `pgx.ErrNoRows` returns `DefaultSettings`, not an error.
- Every read and write repopulates the cache (failures ignored).
- `Save` is an `INSERT ... ON CONFLICT (user_id) DO UPDATE`, with
  `NULLIF($15,'')::time` so empty quiet-hour strings become SQL `NULL`.

### 6.4 `device_repo.go`

- `Register` upserts by token and reactivates it. `COALESCE` keeps the previous
  device name/version when the client sends blanks.
- `Revoke` sets `is_active = FALSE` scoped to the owning user.
- `ListActive` is the push worker query, ordered by `last_seen_at DESC`.
- `MarkInactive` is called by the push worker when FCM/APNs reports an invalid
  token.

### 6.5 `clients.go`

`grpcActorDirectory` hydrates actor IDs with the User Service. Because that
service only exposes `GetUserById`, the adapter implements a bounded batch call:
up to 100 unique IDs, 8 workers, deduplicated up front. Failures are soft — the
map simply omits the actor, and the service falls back to the IDs stored on the
notification.

---

## 7. The service layer

### 7.1 `ports.go`

The service depends only on these interfaces:

- `NotificationStore`
- `SettingsStore`
- `CacheStore`
- `DeviceStore`
- `ActorResolver`
- `RealtimePublisher`
- `PushQueue`
- `EmailQueue`

`repository.NotificationRepository` satisfies `NotificationStore`; the Redis
repository satisfies `CacheStore`; dispatchers satisfy the queue interfaces.
This is what makes the unit tests fast and deterministic.

### 7.2 `options.go`

```go
type Options struct {
    AggregationWindow time.Duration // 15m
    DedupTTL          time.Duration // 60s
    RealtimeThrottle  time.Duration // 30s
    MaxPushPerHour    int           // 5
    PushRateWindow    time.Duration // 1h
    Now               func() time.Time
}
```

`withDefaults` fills zero values. `Now` is injectable so quiet-hours tests do
not depend on wall time.

### 7.3 `aggregator.go`

The aggregator does not own persisted state. Redis maps one entity to one open
notification row:

```text
agg:<recipient>:<type>:<entity>  →  notification UUID
agg:actors:<key>                 →  SET of actor UUIDs
```

- `Existing` loads the open row ID.
- `Remember` stores it for one window.
- `AddActor` returns `false` if the actor is already in the set.
- `RemoveActor` powers unlike handling.
- `ShouldBroadcast` uses `agg:rt:<key>` with a 30-second window so a viral post
  emits at most one realtime frame per 30 seconds.

### 7.4 `policy_service.go`

Three channel checks:

- **In-app** — fails **open** on a settings outage. System alerts always pass.
- **Push** — fails **closed** on a settings outage. System alerts bypass
  category toggles, quiet hours, and rate limiting. Otherwise:
  1. `push_enabled` master switch,
  2. category toggle (`push_likes` etc.),
  3. `IsQuietHours(now)`,
  4. `AllowPush` per-user fixed-window budget. A rate-limiter failure allows the
     push (spam control is not a correctness boundary).
- **Email** — currently gates security alerts; digest batching belongs to the
  email provider.

Quiet hours are evaluated in the user's IANA timezone. Windows that cross
midnight (`22:00–07:00`) work; equal start/end means "all day".

### 7.5 `notification_service.go`

This is the core. Its public methods fall into four groups.

#### Reads

`GetNotifications`:

1. Parse/validate the user ID.
2. Clamp `page_size` to 1–100 (default 20).
3. Parse the optional type filter using the tolerant parser.
4. Decode the opaque cursor.
5. Fetch `pageSize + 1` rows from the store.
6. Trim, compute `has_more`, and encode the last row's
   `(created_at, id)` as the next cursor.
7. Hydrate actor profiles and re-render titles.
8. Return the unread count.

`GetUnreadCount` uses `unreadCount`, which prefers the Redis counter and falls
back to `COUNT(*)`, seeding Redis afterwards.

#### Mutations

- `MarkAsRead` deduplicates incoming IDs, validates every UUID, counts the rows
  that actually changed, and decrements the badge by that exact count. If Redis
  is unavailable, it recomputes the badge from PostgreSQL.
- `MarkAllAsRead` performs one bulk update and resets the Redis counter to zero.
  It returns `marked_count` for the client's optimistic UI.
- `DeleteNotification` soft-deletes, checks ownership in SQL, and decrements
  the badge only when the deleted row was unread.

#### Preferences and devices

- `GetSettings` and `UpdateSettings`. The path `user_id` always overwrites the
  body's `user_id`, so a client cannot modify another user's settings.
- `RegisterDeviceToken` and `RevokeDeviceToken` validate platform/token before
  touching the repository.

#### Write path

`ProcessEvent` is described in §3.5. Internally:

`createNotification`:

1. Records the actor ID (if any) and hydrates its profile.
2. Chooses `entity_type` (`post`, `user`, `comment`, `system`) and a fallback
   `entity_id` for follows/system alerts.
3. Renders the title, preview, and deep link.
4. Inserts the row.
5. Increments the badge.
6. Seeds the aggregation window when applicable.
7. Publishes realtime, enqueues push, enqueues email.

`processAggregated`:

1. Compute the aggregation key.
2. If no open row, create one and remember it.
3. `AddActor`. If the actor is already counted, stop.
4. Load the row. If it disappeared (retention, deletion), forget the key and
   create a new row.
5. Append the actor (max 3 stored), increment the total, re-render the title.
6. `UpdateAggregate(..., reopen=true)`.
7. If the row was read before, increment the badge; otherwise read the current
   counter.
8. Publish a realtime frame only if the throttle allows. Aggregated updates
   **never** enqueue push — one "Alice and 45 others" toast is enough.

`RevokeAggregatedActor` (unlike):

1. Remove the actor from the set. If it was not present, stop.
2. Decrement the total.
3. At zero actors: soft-delete the row, decrement the badge if it was unread,
   and clear the aggregation key.
4. Otherwise: rewrite actor preview/title with `reopen=false` so a retracted
   like cannot un-read an already-read notification.

#### Dispatch helpers

- `publishRealtime` is best-effort. The inbox row is committed before this
  point, so a Pub/Sub failure only means the client learns about it on next
  refresh.
- `enqueuePush` evaluates push policy first. The worker queue is bounded, so
  `ErrQueueFull` is logged and ignored — the inbox is still correct.
- `enqueueEmail` only handles `system_alert`. It resolves the recipient's email
  through the actor directory; if there is none, it logs and skips.

#### Actor hydration

`hydrateNotifications` collects up to three actor IDs per row, performs one
batch lookup, and re-renders titles. Missing profiles fall back to
`models.Actor{ID: id}` so rows always render.

---

## 8. Transport layer

### 8.1 gRPC (`handler/grpc_handler.go`)

Implements every RPC in `proto/notification.proto`:

- `GetNotifications` maps the proto enum to a string filter, calls the service,
  and converts rows to `NotificationItem`.
- `GetUnreadCount`
- `MarkAsRead` rejects an empty ID list with `InvalidArgument`.
- `MarkAllAsRead` returns `marked_count`.
- `DeleteNotification`
- `GetSettings` / `UpdateSettings`
- `RegisterDeviceToken` / `RevokeDeviceToken`

`mapError` translates domain errors to gRPC codes:

| Domain error | gRPC code |
| --- | --- |
| `ErrInvalidInput`, `ErrInvalidCursor` | `InvalidArgument` |
| `ErrNotificationNotFound` | `NotFound` |
| `context.DeadlineExceeded` | `DeadlineExceeded` |
| `context.Canceled` | `Canceled` |
| anything else | `Internal` (details logged, not leaked) |

### 8.2 WebSocket hub (`handler/ws_hub.go`)

The hub owns:

```go
clients map[string]map[*wsClient]struct{} // user → connections
subs    map[string]context.CancelFunc     // user → Redis subscription
```

- The first connection for a user starts one `SUBSCRIBE notify:<user>`.
- Every additional device shares that subscription.
- The last disconnect cancels it.
- Each client has a 32-frame outbound buffer. A slow client is dropped from the
  broadcast rather than blocking the hub.

### 8.3 WebSocket handler (`handler/ws_handler.go`)

- Authenticates with `?token=` or `Authorization: Bearer`.
- If `JWT_SECRET` is configured, verifies HS256 with the standard library:
  header algorithm check, HMAC comparison via `hmac.Equal`, and `exp` check.
- If no secret is configured and `ALLOW_DEV_USER_ID=true` (default outside
  production), accepts `?user_id=<uuid>`.
- Origin checking uses `ALLOWED_ORIGINS`; `*` allows all.
- Sends `{"event":"connected"}` immediately.
- Read pump: 4 KB limit, 60-second read deadline refreshed by pongs, responds to
  `{"action":"ping"}` with `{"event":"pong"}`.
- Write pump: 30-second ping ticker, 10-second write deadline.

The frame shape is:

```json
{
  "event": "notification_received",
  "data": {
    "id": "…",
    "type": "post_like",
    "title": "Alice, Bob, and 5 others liked your post",
    "content_preview": "…",
    "action_url": "socialflow://posts/…",
    "actors": [{"id": "…", "username": "alice", "display_name": "Alice"}],
    "total_actors_count": 7,
    "unread_count": 12,
    "created_at": "2026-11-03T10:00:00Z"
  }
}
```

---

## 9. Dispatchers

### 9.1 Realtime (`realtime_dispatcher.go`)

Publishes the JSON payload to `notify:<user>`. Returns nil when Redis is not
configured. Failures are returned to the caller, which logs them; they never
fail ingestion.

### 9.2 Push (`push_dispatcher.go`, `push_providers.go`)

`PushDispatcher` is a bounded worker pool:

- `Enqueue` never blocks. It returns `ErrQueueFull` if the buffer is full.
- Each worker loads the user's active tokens and sends to every device.
- `retry` performs up to 3 attempts with 100/200/400 ms backoff.
- `ErrTokenUnregistered` short-circuits retries and calls `MarkInactive`.
- An empty token list is a no-op.

Providers:

**LogPushProvider** is the default. It exercises the whole pipeline without
calling an external service.

**FCMProvider** implements FCM HTTP v1:

1. Reads the service-account JSON.
2. Signs an RS256 JWT assertion with `iss`, `scope`, `aud`, `iat`, `exp`.
3. Exchanges it at the token endpoint for an OAuth2 access token.
4. Caches the token until one minute before expiry.
5. Sends `POST /v1/projects/<project>/messages:send`.
6. Maps `404`, `410`, `UNREGISTERED`, and invalid-token `400`s to
   `ErrTokenUnregistered`.

**APNsProvider** implements the token-based HTTP/2 API:

1. Parses the `.p8` EC private key.
2. Signs an ES256 JWT with `kid`, `iss`, `iat`.
3. Caches it for 50 minutes.
4. Sends `POST /3/device/<token>` with `apns-topic`, `apns-push-type: alert`,
   and `apns-priority: 10`.
5. Maps `410` and `BadDeviceToken`/`Unregistered`/`DeviceTokenNotForTopic` to
   `ErrTokenUnregistered`.

Both providers use the standard library only — no Firebase or APNs SDK. JWT
signing helpers (`signRS256`, `signES256`) are shared with the WS auth tests.

### 9.3 Email (`email_dispatcher.go`)

`EmailDispatcher` is a small worker pool (2 workers) with the same bounded
queue semantics. `SMTPEmailProvider` builds a plain-text MIME message,
sanitises CR/LF from the subject to prevent header injection, and uses
`smtp.SendMail`, which negotiates STARTTLS automatically when advertised.

`LogEmailProvider` is the default.

---

## 10. Bootstrap (`cmd/server/main.go`)

Startup order:

1. Load config, install a signal-aware context.
2. **PostgreSQL is mandatory** — connect or exit.
3. **Redis is optional** — log a warning and continue in degraded mode.
4. Build repositories. The cache repository is only constructed when Redis is
   available.
5. Build the actor directory (User Service gRPC). Dial failures are warnings.
6. Build dispatchers:
   - realtime (nil-safe),
   - push provider from config with a log fallback on misconfiguration,
   - email provider from config with a log fallback.
7. Start push and email worker pools.
8. Build the notification service with all dependencies.
9. Start the event consumer if Redis is available and ingestion is enabled.
10. Run partition maintenance synchronously; schedule it every 24 hours.
11. Start the gRPC server with the standard health service.
12. Start the WebSocket HTTP server on `WS_PORT` with `/ws` and `/healthz`.
13. Serve until SIGINT/SIGTERM, then:
    - mark gRPC health `NOT_SERVING`,
    - `GracefulStop`,
    - shut down HTTP with a 10-second timeout,
    - wait for dispatcher workers,
    - close the hub and connections.

`maintainPartitions` is best-effort: a transient database error is logged and
retried on the next tick. It calls `EnsurePartitions(ctx, 2)`, drops fully
expired partitions, then prunes the partially expired month.

---

## 11. Failure and degradation matrix

| Dependency | Failure | Behaviour |
| --- | --- | --- |
| PostgreSQL | down | service cannot start / requests fail |
| Redis | down at boot | service starts; unread counts fall back to SQL; dedup, aggregation, throttling, rate limits, realtime disabled |
| Redis | down at runtime | each operation returns an error; service logs and continues |
| User Service | down | notifications still persist; actor names fall back to stored IDs |
| FCM / APNs | 5xx/timeout | 3 retries, then log; inbox is unaffected |
| FCM / APNs | invalid token | token deactivated, no retry |
| SMTP | down | 2 retries, then log; inbox is unaffected |
| WebSocket Pub/Sub | publish fails | logged; client refreshes inbox later |
| Consumer | transient handler error | message stays pending; `XAutoClaim` retries |
| Consumer | poison message | DLQ after `EVENT_MAX_ATTEMPTS`, then ack |

The invariant is: **the durable inbox write happens before any dispatch**. A
channel failure never loses a notification.

---

## 12. Tests

All tests are hermetic; they do not need PostgreSQL, Redis, or network access.

| Package | File | Coverage |
| --- | --- | --- |
| `models` | `notification_test.go` | type parsing, title/preview rendering, dedup keys, validation, cursors, settings, quiet hours |
| `models` | `events_test.go` | all three stream layouts, normalization, payload recipient resolution |
| `repository` | `cache_repo_test.go` | miniredis counters, floor-at-zero Lua, dedup, aggregation actors, throttle, push budget, degraded mode |
| `services` | `notification_service_test.go` | create, self-action drop, dedup, aggregation, same-actor dedup, unlike, mark-read, delete, pagination, filter, quiet hours, security-alert bypass, settings ownership, device validation |
| `events` | `event_handler_test.go` | every event mapping, permanent errors, unfollow no-op |
| `handler` | `grpc_handler_test.go` | proto mapping, argument validation, error codes, settings round trip |
| `handler` | `ws_handler_test.go` | HS256 verification, expiry, wrong algorithm/secret, dev auth, UUID validation |
| `dispatchers` | `push_providers_test.go` | fan-out, stale-token deactivation, queue bounds, worker delivery, RS256/ES256 signing, key parsing |

Run:

```bash
cd notification-service
go test ./...
go vet ./...
```

Note: `go test -race` requires cgo and is not available on every Windows
toolchain; the suite is written to avoid shared-state races (fakes use mutexes).

---

## 13. Configuration reference

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `50055` | gRPC API port |
| `WS_PORT` | `8085` | WebSocket/HTTP port |
| `ENV` | `development` | gates dev auth default |
| `DATABASE_URL` | local `socialflow_notifications` | PostgreSQL DSN |
| `DB_MAX_OPEN_CONNS` | `25` | pool size |
| `DB_MAX_IDLE_CONNS` | `10` | minimum idle connections |
| `DB_CONN_MAX_LIFETIME` | `5m` | connection recycle |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD`, `REDIS_DB` | empty / `0` | Redis auth/db |
| `EVENT_CONSUMER_ENABLED` | `true` | enable ingestion |
| `EVENT_STREAM` | `social:events` | comma-separated streams |
| `EVENT_GROUP` | `notification-service` | consumer group |
| `EVENT_CONSUMER_NAME` | hostname-PID | consumer name |
| `EVENT_CLAIM_INTERVAL` | `30s` | reclaim idle messages |
| `EVENT_MAX_ATTEMPTS` | `5` | DLQ threshold |
| `EVENT_DLQ_STREAM` | `stream:notifications:dlq` | dead-letter stream |
| `USER_SERVICE_ADDR` | `localhost:50051` | actor hydration |
| `AGGREGATION_WINDOW` | `15m` | like roll-up window |
| `DEDUP_TTL` | `60s` | duplicate suppression |
| `REALTIME_THROTTLE` | `30s` | aggregate frame throttle |
| `MAX_PUSH_PER_HOUR` | `5` | push budget |
| `PUSH_RATE_WINDOW` | `1h` | budget window |
| `UNREAD_CACHE_TTL` | `168h` | badge key TTL |
| `RETENTION_DAYS` | `90` | partition/row retention |
| `JWT_SECRET` | empty | HS256 WS auth secret |
| `ALLOW_DEV_USER_ID` | `true` outside production | `?user_id=` auth |
| `ALLOWED_ORIGINS` | `*` | WS origin allowlist |
| `PUSH_PROVIDER` | `log` | `log`, `fcm`, `apns` |
| `FCM_CREDENTIALS_FILE`, `FCM_PROJECT_ID` | empty | FCM config |
| `APNS_KEY_FILE`, `APNS_KEY_ID`, `APNS_TEAM_ID`, `APNS_TOPIC` | empty / app bundle | APNs config |
| `APNS_SANDBOX` | `false` | sandbox endpoint |
| `EMAIL_PROVIDER` | `log` | `log`, `smtp` |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `EMAIL_FROM` | empty / `587` / local sender | SMTP config |
| `WORKER_COUNT` | `8` | push workers |
| `QUEUE_SIZE` | `1024` | push queue size (email uses 1/4) |

---

## 14. Protobuf contract

`proto/notification.proto` defines:

- `NotificationService` with 9 RPCs.
- `NotificationType` enum (unspecified, follow, post_like, comment,
  comment_reply, mention, creator_post, system_alert).
- `NotificationItem`, `NotificationActor`, page/request/response messages.
- `NotificationSettings` and device token registration messages.

Generated files:

```bash
cd proto
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       notification.proto
```

The `MarkAllAsReadResponse` includes `marked_count`, which is useful for
optimistic UI updates.

---

## 15. Running locally

```bash
# 1. PostgreSQL
createdb socialflow_notifications

# 2. Schema
cd notification-service
cp .env.example .env
make migrate-up

# 3. Optional: publish a test event
redis-cli XADD social:events '*' type post.liked post_id <post> \
  post_author_id <author> actor_id <actor>

# 4. Run
make run
# or: go run ./cmd/server
```

The service exposes:

- gRPC on `:50055`
- WebSocket on `:8085/ws?token=<jwt>` (or `?user_id=` in development)
- health check on `:8085/healthz`

With the default `log` providers, push and email deliveries are printed to
stdout while the full worker/token/policy pipeline still runs. Set
`PUSH_PROVIDER=fcm|apns` and `EMAIL_PROVIDER=smtp` for real delivery.

---

## 16. Design decisions and deviations

1. **Redis Streams instead of Kafka.** Consistent with `feed-service`; the
   consumer is isolated behind `models.Event`, so the transport can be replaced
   without touching business logic.
2. **Monthly partitions.** Retention becomes `DROP TABLE` instead of a massive
   `DELETE`. The composite primary key is a PostgreSQL requirement.
3. **`TEXT[]` for `actor_ids`.** Simpler pgx scanning; values are validated UUID
   strings.
4. **Aggregation limited to likes.** Comments and mentions carry distinct
   content, so collapsing them would lose information.
5. **Actor-set guard.** The short dedup TTL can expire while an aggregation
   window is still open; the Redis set prevents the same actor being counted
   twice.
6. **Unfollow is a no-op.** Removing a historical "started following you" row
   would rewrite history; unlike, by contrast, is a correction of an aggregate.
7. **Push and email are in-process worker pools.** This keeps the deployment
   simple. At larger scale the `PushQueue`/`EmailQueue` interfaces can be
   backed by a durable queue without changing the service.
8. **Standard-library JWT/crypto.** Avoids extra dependencies for WS auth,
   FCM assertions, and APNs tokens.

---

## 17. Suggested commit map

The implementation is grouped into coherent commits:

| Commit | Contents |
| --- | --- |
| `feat(proto): add notification service contract` | `notification.proto` + generated stubs, `go.work` entry |
| `feat(notification-service): add module scaffolding and configuration` | `go.mod`, `.env.example`, `.air.toml`, `makefile`, config, database |
| `feat(notification-service): add inbox, settings, and device migrations` | all three migration pairs |
| `feat(notification-service): add domain models and event schemas` | models package |
| `feat(notification-service): add PostgreSQL, Redis, and user-service repositories` | repository package |
| `feat(notification-service): implement policy, aggregation, and notification orchestration` | services package |
| `feat(notification-service): add realtime, push, and email dispatchers` | dispatchers package |
| `feat(notification-service): add stream consumer and event router` | events package |
| `feat(notification-service): add gRPC and WebSocket transports` | handler package |
| `feat(notification-service): wire server bootstrap and graceful shutdown` | `cmd/server/main.go` |
| `test(notification-service): cover models, services, and transports` | all `_test.go` files |
| `docs(notification-service): add full code walkthrough` | this file |

---

## 18. Quick reference: file responsibilities

| File | Responsibility |
| --- | --- |
| `cmd/server/main.go` | dependency wiring, servers, workers, partitions, shutdown |
| `internal/config/config.go` | env parsing with production-safe defaults |
| `internal/database/postgres.go` | pgx pool + ping |
| `internal/database/redis.go` | Redis client + ping |
| `internal/models/errors.go` | sentinel errors shared across layers |
| `internal/models/notification.go` | type enum, rendering, dedup key, validation |
| `internal/models/cursor.go` | opaque keyset cursor |
| `internal/models/settings.go` | preferences, quiet hours, defaults |
| `internal/models/device.go` | device token validation |
| `internal/models/events.go` | event envelope, normalization, producer schemas |
| `internal/models/dispatcher.go` | realtime/push/email message shapes |
| `internal/repository/notification_repo.go` | inbox SQL, partitions, retention |
| `internal/repository/cache_repo.go` | counters, dedup, aggregation, limits |
| `internal/repository/settings_repo.go` | settings SQL + JSON cache |
| `internal/repository/device_repo.go` | device token SQL |
| `internal/repository/clients.go` | User Service actor hydration |
| `internal/services/ports.go` | service dependency interfaces |
| `internal/services/options.go` | behavioural defaults |
| `internal/services/aggregator.go` | Redis aggregation state |
| `internal/services/policy_service.go` | preferences, quiet hours, push budget |
| `internal/services/notification_service.go` | orchestration and read/mutation API |
| `internal/events/consumer.go` | group reads, reclaim, retries, DLQ |
| `internal/events/event_handler.go` | producer event → domain request |
| `internal/dispatchers/realtime_dispatcher.go` | Redis Pub/Sub publish |
| `internal/dispatchers/push_dispatcher.go` | push worker pool + retry |
| `internal/dispatchers/push_providers.go` | log, FCM v1, APNs, JWT helpers |
| `internal/dispatchers/email_dispatcher.go` | email worker pool, log, SMTP |
| `internal/handler/grpc_handler.go` | gRPC API + error mapping |
| `internal/handler/ws_hub.go` | per-user connections and subscriptions |
| `internal/handler/ws_handler.go` | upgrade, JWT auth, ping/pong |
| `migrations/*.sql` | partitioned inbox, settings, device tokens |
| `proto/notification.proto` | public contract |

---

## 19. What to look at first

- **Want to understand ingestion?** Start at `events/consumer.go`, then
  `events/event_handler.go`, then `services/notification_service.go`.
- **Want to change aggregation?** `services/aggregator.go` and
  `processAggregated` in `notification_service.go`.
- **Want to add a channel?** Implement `PushProvider`/`EmailProvider` and wire
  it in `buildPushProvider`/`buildEmailProvider` in `main.go`.
- **Want to add an event?** Add a payload struct and a `NormalizeEventType`
  case in `models/events.go`, then a handler in `events/event_handler.go`.
- **Want to change the inbox schema?** Add a migration and update
  `notificationColumns`/`scanNotification` in `notification_repo.go`.

That is the whole service: a durable partitioned inbox, a Redis hot path that
can disappear without taking the service down, an at-least-once consumer with a
DLQ, and three independent delivery channels behind one policy engine.
