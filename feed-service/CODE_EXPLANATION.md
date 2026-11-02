# Feed Service — Complete Code & Architecture Walkthrough

This document explains every part of `feed-service`: what each file does, how a feed request flows through the system, how the hybrid fanout model is implemented, how cursor pagination stays stable, and how the service degrades when Redis or a downstream service is unavailable.

The folder layout intentionally mirrors `post-service`:

```
cmd/server/main.go          -> bootstrap + dependency injection
internal/config             -> environment configuration
internal/database           -> Redis connection
internal/handler            -> gRPC transport
internal/models             -> domain models and errors
internal/repository         -> Redis timeline + gRPC adapters
internal/services           -> read path + fanout business logic
internal/events             -> Redis Streams event consumer
```

---

## 1. What the service does

The Feed Service owns three things:

1. **A precomputed home timeline per user** — a Redis Sorted Set containing only post IDs and timestamps.
2. **The hybrid fanout policy** — normal authors are pushed to followers, celebrities are pulled at read time.
3. **Feed hydration** — turning a page of post IDs into complete post cards by calling `post-service`, with cursor pagination and ghost-post filtering.

It does **not** own users, posts, or the follow graph. Those remain in `user-service`, `post-service`, and `follow-service`.

### Design targets

| Goal | Implementation |
| --- | --- |
| Low read latency | Redis `ZREVRANGEBYSCORE` over a per-user sorted set |
| No feed drift | Cursor pagination based on `(timestamp, post_id)` |
| No write amplification for celebrities | Author-level shared cache pulled at read time |
| Graceful degradation | Empty/unavailable cache falls back to live pull aggregation |
| No N+1 round trips | Bounded concurrent hydration worker pool |
| No ghost posts | Missing/private posts are filtered and asynchronously pruned |
| Dormant-user protection | Activity marker + TTL; fanout skips inactive followers |

---

## 2. Folder structure

```
feed-service/
├── cmd/
│   └── server/
│       └── main.go                  # config, Redis, gRPC clients, server, consumer
├── internal/
│   ├── config/
│   │   └── config.go                # typed environment configuration
│   ├── database/
│   │   └── redis.go                 # pooled Redis client + Ping
│   ├── events/
│   │   └── consumer.go              # Redis Streams consumer group
│   ├── handler/
│   │   ├── feed_handler.go          # FeedService gRPC implementation
│   │   └── feed_handler_test.go
│   ├── models/
│   │   ├── errors.go                # sentinel errors
│   │   ├── events.go                # event envelope + payloads + score helpers
│   │   ├── events_test.go
│   │   └── feed.go                  # FeedPost, GetFeedQuery, FeedResult, TimelineItem
│   ├── repository/
│   │   ├── clients.go               # Follow/Post gRPC adapters
│   │   ├── feed_repository.go       # repository facade
│   │   ├── timeline_cache.go        # Redis sorted-set operations
│   │   └── timeline_cache_test.go
│   └── services/
│       ├── options.go               # shared policy knobs and defaults
│       ├── feed_service.go          # read path
│       ├── fanout_service.go        # write path
│       ├── feed_service_test.go
│       └── test_helpers_test.go
├── .air.toml
├── .env.example
├── .gitignore
├── go.mod / go.sum
├── makefile
├── SYSTEM_DESIGN.md                # original architecture brief
└── CODE_EXPLANATION.md             # this document
```

---

## 3. Configuration (`internal/config/config.go`)

`config.Load()` reads `.env` when present, then falls back to process environment variables and finally to defaults.

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `50054` | gRPC listen port |
| `FOLLOW_SERVICE_ADDR` | `localhost:50053` | Follow Service target |
| `POST_SERVICE_ADDR` | `localhost:50052` | Post Service target |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | empty | Redis auth |
| `REDIS_DB` | `0` | Redis logical database |
| `TIMELINE_MAX_SIZE` | `800` | Max post IDs retained per user |
| `TIMELINE_TTL_HOURS` | `720` | Timeline key TTL (30 days) |
| `CELEBRITY_THRESHOLD` | `25000` | Follower count that switches to pull |
| `INACTIVE_THRESHOLD_DAYS` | `14` | Activity-marker TTL used for fanout filtering |
| `EVENT_CONSUMER_ENABLED` | `true` | Start the Redis Streams consumer |
| `EVENT_STREAM` | `social:events` | Stream name |
| `EVENT_GROUP` | `feed-service` | Consumer group |
| `EVENT_CONSUMER_NAME` | `<host>-<pid>` | Consumer name inside the group |

The helpers (`getEnv`, `getEnvAsInt`, `getEnvAsInt64`, `getEnvAsBool`) silently fall back to defaults when a value is missing or malformed, so a typo cannot prevent the service from booting.

---

## 4. Domain models (`internal/models`)

### `feed.go`

- **`FeedPost`** — the hydrated post card returned over gRPC. It mirrors the fields available in `proto/post.proto` and `proto/feed.proto`.
- **`GetFeedQuery`** — `user_id`, `page_size`, `cursor`.
- **`FeedResult`** — `posts`, `next_cursor`, `has_more`.
- **`TimelineItem`** — `post_id` plus an `int64` score in **Unix microseconds**.

Why microseconds rather than nanoseconds?

- A Redis sorted-set score is an IEEE-754 `float64`.
- `float64` represents every integer exactly up to `2^53` (~9.0 × 10¹⁵).
- Current Unix **nanoseconds** (~1.7 × 10¹⁸) lose precision.
- Current Unix **microseconds** (~1.7 × 10¹⁵) are exact and remain exact until roughly the year 2255.

This matters because cursor pagination compares timestamps. Losing precision would make page boundaries nondeterministic.

### `errors.go`

| Error | Meaning |
| --- | --- |
| `ErrInvalidInput` | Bad UUID, empty payload, unknown event |
| `ErrInvalidCursor` | Cursor is not valid base64 or not `<score>:<post_id>` |
| `ErrFeedUnavailable` | Follow/Post Service could not produce a feed |
| `ErrCacheUnavailable` | Redis is not configured or unreachable |

### `events.go`

Event types:

```text
post.created
post.deleted
user.followed
user.unfollowed
```

The broker envelope is intentionally transport-neutral:

```json
{
  "type": "post.created",
  "payload": {
    "post_id": "…",
    "author_id": "…",
    "created_at": 1793620800000,
    "is_public": true
  }
}
```

`EventFromValues` accepts either:

- one field named `event` containing the full JSON envelope, or
- two fields `type` and `payload`.

That makes the consumer compatible with simple `XADD` commands.

`ScoreFromTimestamp` normalises an unknown Unix precision (seconds, milliseconds, microseconds, or nanoseconds) into microseconds. This prevents a producer sending `time.Now().Unix()` from creating a timeline dated in 1970.

---

## 5. Redis connection (`internal/database/redis.go`)

`Connect(addr, password, db)`:

- creates a `go-redis/v9` client,
- uses a 50-connection pool with 10 idle connections,
- sets 2–3 second dial/read/write timeouts,
- verifies connectivity with `PING`,
- closes the client and returns an error if `PING` fails.

The caller (`main.go`) treats Redis as optional. If Redis is unavailable:

- the service still starts,
- reads fall back to live pull aggregation,
- the event consumer is not started.

---

## 6. Timeline cache (`internal/repository/timeline_cache.go`)

This is the only file that knows Redis command syntax.

### Key schema

| Key | Type | Contents |
| --- | --- | --- |
| `timeline:<user_id>` | ZSET | score = post time in Unix microseconds, member = post ID |
| `celebrity:recent:<author_id>` | ZSET | latest public posts of a high-follower author |
| `user:active:<user_id>` | STRING | activity marker with TTL |

Only IDs are stored. Full post payloads are never duplicated into feeds.

### Operations

| Method | Redis commands | Used by |
| --- | --- | --- |
| `GetTimeline` | `ZREVRANGEBYSCORE key max -inf LIMIT 0 n` | read path |
| `PushToTimeline` | `ZADD`, `ZREMRANGEBYRANK`, `EXPIRE` (pipeline) | author self-feed |
| `PushBatch` | chunked pipelines of the same commands | standard fanout |
| `BackfillTimeline` | `ZADD` many, trim, expire | follow events / cold start |
| `RemovePost` | `ZREM` | eager delete |
| `RemovePostsBatch` | pipelined `ZREM` | unfollow cleanup |
| `InvalidateTimeline` | `DEL` | explicit invalidation |
| `StoreCelebrityPost` | `ZADD`, trim, expire | celebrity write |
| `GetCelebrityRecentPostsBatch` | pipelined `ZREVRANGE ... WITHSCORES` | hybrid read merge |
| `RemoveCelebrityPost` | `ZREM` | celebrity delete |
| `MarkUserActive` | `SET key 1 EX ttl` | every feed read |
| `IsUserActiveBatch` | pipelined `EXISTS` | fanout filtering |
| `Ping` | `PING` | startup / health |

### Trimming

Timelines are capped at `TIMELINE_MAX_SIZE` (default 800). Because a ZSET is ordered ascending by score, the oldest entries live at ranks `0..n`. This removes everything except the newest `maxSize` members:

```text
ZREMRANGEBYRANK timeline:<user> 0 -(maxSize + 1)
```

### Batch chunking

`PushBatch` processes followers in chunks of 500. A user with 20,000 followers produces 40 pipelines instead of 20,000 individual connections. This keeps memory bounded and avoids one giant command buffer.

### No-op fallback

`NewRedisTimelineCache(nil)` returns `noopTimelineCache`:

- reads return `ErrCacheUnavailable` so the service falls back to pull mode,
- writes return `ErrCacheUnavailable` so an event consumer does **not** acknowledge a fanout that was never written,
- removals and activity markers are safe no-ops.

---

## 7. Downstream gRPC adapters (`internal/repository/clients.go`)

### Follow client

```go
GetFollowingUserIDs(ctx, userID)  -> follow-service.GetFollowing
GetFollowerUserIDs(ctx, userID)   -> follow-service.GetFollowers
GetFollowCounts(ctx, userID)      -> follow-service.GetFollowCounts
```

Every call gets a 3-second child context (`rpcTimeout`).

### Post client

```go
GetPostByID(ctx, postID)          -> post-service.GetPostById
GetPostsByAuthor(ctx, authorID)   -> post-service.GetAllPosts
GetPostsByAuthorIDs(ctx, ids)     -> bounded worker pool over GetPostsByAuthor
HydratePosts(ctx, postIDs)        -> bounded worker pool over GetPostByID
```

`GetPostByID` maps gRPC `NotFound` to `(nil, nil)`. This is an intentional signal: the post was deleted, archived, or became inaccessible. Other errors remain errors, so transient outages are not mistaken for deletions.

`HydratePosts` returns three values:

1. hydrated posts in the same order as the requested IDs,
2. IDs that Post Service explicitly reported as missing,
3. an error only if the whole page failed.

`missing` is the ghost list. Transient RPC failures are **not** added to `missing`, so the read path does not delete cache entries just because Post Service had a bad second.

Current Post Service exposes single-post and per-author RPCs, not `BatchGetPosts`. The adapter therefore provides batch semantics with:

- 15 concurrent hydration workers,
- 10 concurrent author workers for cold start.

If a real `BatchGetPostsByIds` RPC is added to `proto/post.proto`, only `HydratePosts` needs to change; the service layer stays untouched.

### Timestamp parsing

Post Service returns RFC3339 strings. `toFeedPost` tries `RFC3339Nano` first, then `RFC3339`. `updated_at` is optional and becomes `nil` when absent.

---

## 8. Repository facade (`internal/repository/feed_repository.go`)

`FeedRepository` hides whether data came from Redis, Follow Service, or Post Service. The service layer depends only on this interface:

```go
TimelineCache() TimelineCache
GetFollowingUserIDs(...)
GetFollowerUserIDs(...)
GetFollowCounts(...)
GetPostsByAuthor(...)
GetPostsByAuthorIDs(...)
HydratePosts(...)
```

Two constructors exist:

- `NewFeedRepository(followClient, postClient, redisClient)` for production.
- `NewFeedRepositoryWithClients(cache, followClient, postClient)` for tests and alternate transports.

---

## 9. Read path (`internal/services/feed_service.go`)

### Entry point

```go
GetFeed(ctx, models.GetFeedQuery) (*models.FeedResult, error)
```

### Step-by-step

```
Client
  │
  ▼
Validate user UUID + parse cursor
  │
  ▼
Mark user active (refresh 14-day activity key)
  │
  ▼
Read timeline:<user> from Redis
  │
  ├── Redis error / empty ──────────────► coldStart()
  │                                         │
  │                                         ├── GetFollowing
  │                                         ├── GetAllPosts for each author + self
  │                                         ├── filter private posts
  │                                         ├── merge, dedupe, sort
  │                                         ├── async warm Redis
  │                                         └── paginate with cursor filter
  │
  ▼
Filter out cursor item and same-score ties
  │
  ▼
Pull followed celebrities' recent caches (pipelined)
  │
  ▼
Merge + dedupe + sort by (score DESC, post_id DESC)
  │
  ▼
Take pageSize + 1 candidates
  │
  ▼
Hydrate via Post Service (bounded concurrency)
  │
  ▼
Drop missing/deleted + private posts
  │
  ▼
Async prune ghosts from Redis
  │
  ▼
Build next_cursor from the last returned timeline item
```

### Page size

- `page_size <= 0` → `20`
- `page_size > 100` → `100`

### Reading the cache

```go
fetchLimit := int64(pageSize)*2 + 2
items := cache.GetTimeline(ctx, userID, cursorScore, fetchLimit)
```

The cache is deliberately over-fetched. Reasons:

- the cursor item itself is included because the Redis max score is inclusive,
- multiple posts can share the same microsecond,
- deleted/private posts may consume slots.

`GetTimeline` returns newest-first because it uses `ZREVRANGEBYSCORE`.

### Cursor filtering

A cursor is:

```text
base64url("<unix_microseconds>:<post_id>")
```

Example decoded value:

```text
1793620800123456:5f0c2a2e-...
```

To continue strictly after the last returned post:

```go
item.Score < cursor.Score ||
(item.Score == cursor.Score && item.PostID < cursor.PostID)
```

The ID tie-break is descending because the feed itself is sorted with `post_id DESC` when scores are equal. The cursor's post ID is also skipped explicitly, which protects against a cache expiring between pages and the post timestamp differing by a few microseconds from the event timestamp.

### Celebrity merge

Celebrity posts are **not** in the follower's `timeline:` key. On every read:

1. `follow-service.GetFollowing` returns current followed IDs.
2. The cache pipelines `ZREVRANGE` over `celebrity:recent:<author>` for all of them.
3. Results are filtered by the cursor.
4. Results are merged with pushed timeline items, deduped by post ID, and sorted.

This avoids one `GetFollowCounts` call per followed account on every page. The presence of a `celebrity:recent:` key is the classification signal.

### Hydration

`buildPage`:

1. Collects candidate post IDs.
2. Calls `HydratePosts`.
3. Builds an ID → post map.
4. Iterates candidates in order (so ordering is preserved).
5. Drops posts that Post Service did not return.
6. Drops posts that are private and authored by someone else.
7. Sends missing/private IDs to `pruneGhostsAsync`.

A user can always see their own private posts; other users cannot.

### Ghost pruning

Pruning runs in a detached goroutine with a 3-second timeout, so a slow Redis cleanup never delays the response:

```go
go func() {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    _ = cache.RemovePostsBatch(ctx, userID, postIDs)
}()
```

### Pagination result

`has_more` is true when:

- more than `pageSize` candidates were available, or
- ghosts consumed part of a full candidate window.

If every candidate turned out to be a ghost, `has_more` is forced to false and no cursor is returned. This prevents clients from looping on an empty page.

The next cursor uses the **timeline score**, not the hydrated post timestamp:

```go
nextCursor = encodeCursorFromScore(validScores[lastIndex], valid[lastIndex].ID)
```

Why? An event's `created_at` and the database's `created_at` can differ by microseconds. Using the actual cache score guarantees the next query resumes strictly after the last returned entry.

### Cold start / pull fallback

`coldStart` runs when:

- Redis returns an error,
- the timeline is empty,
- the cache is unavailable at startup.

It:

1. Fetches followed user IDs.
2. Always adds the requesting user (their own posts belong in their feed).
3. Fetches posts for all authors concurrently.
4. Filters private posts from other authors.
5. Dedupes by post ID and sorts descending.
6. Asynchronously backfills the newest `TIMELINE_MAX_SIZE` posts into Redis.
7. Applies the cursor filter.
8. Returns one page and the next cursor.

This is the original pull model from the design document. It is slower than a cache hit but keeps the service functional with Redis down.

### Async cache warming

`warmCacheAsync` copies the newest posts into `TimelineItem` values and writes them with a 5-second detached context. The copy is important: the caller continues to slice/filter its own `posts` slice while the goroutine runs.

### Invalidation

`InvalidateUserFeed` deletes `timeline:<user_id>`. The next read rebuilds through `coldStart`. If the cache is a no-op (Redis absent), invalidation returns success because there is nothing stale to remove.

---

## 10. Write path (`internal/services/fanout_service.go`)

`FanoutService` exposes one method per event plus a dispatcher:

```go
FanoutPost(...)
HandlePostDeleted(...)
HandleUserFollowed(...)
HandleUserUnfollowed(...)
HandleEvent(ctx, models.Event)
```

### `FanoutPost` — hybrid routing

```
PostCreated event
      │
      ▼
Validate post ID + author ID
      │
      ▼
Push post into author's own timeline
      │
      ├── private? ──► stop
      │
      ▼
Get author follower count
      │
      ├── >= CELEBRITY_THRESHOLD
      │       └── Store in celebrity:recent:<author>, stop
      │
      ▼
Get follower IDs
      │
      ▼
Pipelined activity check (user:active:<id>)
      │
      ▼
Push to active followers only (chunked pipelines)
```

Important decisions:

- The author is always pushed first, even for private posts.
- Private posts are never distributed.
- `>= threshold` uses the pull model.
- Inactive followers are skipped. Their cache is rebuilt on demand when they next read. This prevents wasting memory on dormant accounts.
- If the activity marker is missing for every follower (fresh deployment), no push happens. Reads still work via cold start. This is deliberate: the alternative (pushing to everyone when activity data is absent) defeats the inactive-user policy.

### `HandlePostDeleted`

Removes the post from:

- the author's `timeline:` key,
- the author's `celebrity:recent:` key.

Follower timelines are not enumerated. They are cleaned lazily during read hydration (ghost pruning). This keeps deletion O(1) with respect to follower count.

### `HandleUserFollowed`

1. Marks the follower active.
2. Fetches the followed author's posts.
3. Keeps only public posts.
4. Sorts newest-first.
5. Caps at 50 posts.
6. Backfills the follower's timeline.

### `HandleUserUnfollowed`

1. Fetches the unfollowed author's posts.
2. Removes all known post IDs from the follower's timeline with one pipeline.

Any IDs not returned by Post Service remain and are removed lazily if they are ever hydrated.

### `HandleEvent`

Decodes the payload and dispatches by event type. Unknown types return `ErrInvalidInput` so poison messages are visible in logs.

---

## 11. Event consumer (`internal/events/consumer.go`)

The service subscribes to a Redis Stream consumer group.

### Why Redis Streams?

The production design allows Kafka or RabbitMQ. This repository has no broker infrastructure, so Redis Streams provide:

- at-least-once delivery,
- consumer groups,
- pending-message tracking,
- no extra service to run.

The domain layer sees only `models.Event`. Swapping to Kafka means writing another adapter that calls `FanoutService.HandleEvent`.

### Consumer loop

```text
ensure group (XGROUP CREATE ... MKSTREAM)
  │
  ▼
XREADGROUP GROUP feed-service BLOCK 2000 STREAMS social:events >
  │
  ▼
for each message:
    decode envelope
    │
    ├── malformed ──► XACK (drop poison)
    │
    ├── handler error ──► leave pending (retry later)
    │
    └── success ──► XACK
```

Every 30 seconds, `reclaimPending` calls `XAUTOCLAIM` for messages idle longer than 30 seconds and retries them. This covers transient Post/Follow Service outages.

### Publishing an event

Using `redis-cli`:

```bash
redis-cli XADD social:events '*' \
  type post.created \
  payload '{"post_id":"<uuid>","author_id":"<uuid>","created_at":1793620800000,"is_public":true}'
```

Or with a single envelope field:

```bash
redis-cli XADD social:events '*' \
  event '{"type":"post.created","payload":{"post_id":"<uuid>","author_id":"<uuid>","created_at":1793620800000,"is_public":true}}'
```

`created_at` may be seconds, milliseconds, microseconds, or nanoseconds. `ScoreFromTimestamp` normalises it.

---

## 12. gRPC handler (`internal/handler/feed_handler.go`)

`FeedHandler` embeds `feedpb.UnimplementedFeedServiceServer` and implements:

### `GetFeed`

- rejects an empty `user_id` with `InvalidArgument`,
- calls the domain service,
- converts `time.Time` to RFC3339 strings,
- returns `posts`, `next_cursor`, `has_more`.

### `InvalidateUserFeed`

- rejects an empty `user_id`,
- calls the domain service,
- returns `success: true`.

### Error mapping

| Domain error | gRPC code |
| --- | --- |
| `ErrInvalidInput`, `ErrInvalidCursor` | `InvalidArgument` |
| `ErrFeedUnavailable` | `Unavailable` |
| anything else | `Internal` |

---

## 13. Server bootstrap (`cmd/server/main.go`)

Order of operations:

1. Load config.
2. Create a signal-aware context (`SIGINT` / `SIGTERM`).
3. Connect to Redis. On failure, log a warning and continue.
4. Create gRPC clients for Follow and Post services.
5. Build the repository facade.
6. Build `FanoutService` and `FeedService` with shared `Options`.
7. Start the Redis Streams consumer when Redis is available and enabled.
8. Register `FeedServiceServer`.
9. Listen on `PORT`.
10. On shutdown signal, call `grpcServer.GracefulStop()`.
11. `Serve` until stopped; `grpc.ErrServerStopped` is treated as a clean exit.

The bootstrap uses constructor injection exactly like `post-service`, so every dependency is explicit and testable.

---

## 14. Redis data flow examples

### Normal author creates a post

Assume author `A` has followers `F1` (active) and `F2` (inactive), and follower count is 500.

```text
ZADD timeline:A <score> post-1
EXPIRE timeline:A 30d

GET user:active:F1        -> exists
GET user:active:F2        -> missing

ZADD timeline:F1 <score> post-1
ZREMRANGEBYRANK timeline:F1 0 -801
EXPIRE timeline:F1 30d
```

`F2` gets nothing. When `F2` next opens the feed:

```text
GET timeline:F2           -> empty
-> coldStart
-> follow-service.GetFollowing(F2)
-> post-service.GetAllPosts(A)
-> return page
-> asynchronous ZADD backfill
```

### Celebrity creates a post

Assume author `C` has 2,000,000 followers.

```text
ZADD timeline:C <score> post-9
ZADD celebrity:recent:C <score> post-9
ZREMRANGEBYRANK celebrity:recent:C 0 -21
EXPIRE celebrity:recent:C 30d
```

No follower inbox is touched. When follower `F` reads:

```text
GET timeline:F
-> ZREVRANGE celebrity:recent:C WITHSCORES
-> merge, dedupe, sort
-> hydrate, return
```

### User unfollows an author

```text
post-service.GetAllPosts(author)
-> ZREM timeline:follower post-1 post-2 ...
```

### Post is deleted

```text
ZREM timeline:author post-1
ZREM celebrity:recent:author post-1
```

Follower timelines still contain the ID until their next read. Hydration returns `NotFound`, the read path drops the card, and `RemovePostsBatch` prunes it.

---

## 15. Failure modes and how they are handled

### Redis is down at startup

- `database.Connect` fails.
- `redisClient` stays `nil`.
- The repository uses `noopTimelineCache`.
- Reads always take the cold-start pull path.
- The event consumer is not started.
- The gRPC server still serves requests.

### Redis goes down after startup

- Reads receive an error from `GetTimeline` and fall back to cold start.
- Fanout writes return errors, so the stream consumer leaves events pending.
- `XAUTOCLAIM` retries them after Redis recovers.

### Follow Service is down

- A cache-hit read still returns pushed posts; the celebrity merge is silently skipped.
- A cold-start read returns `Unavailable` because it cannot discover followed authors.
- Fanout returns an error and leaves the event pending.

### Post Service is down

- Hydration fails. If no posts were hydrated, the read returns `Unavailable`.
- If only some posts failed, the page returns the successful cards. Failed IDs are **not** pruned because the failure is transient.
- Deleted posts (`NotFound`) are pruned.

### A post becomes private

- It remains in follower caches until the next hydration.
- The read path sees `is_public = false` and a different author, drops it, and prunes it.

### A user follows someone new

- The follow event backfills up to 50 recent public posts.
- The next read also performs a live celebrity merge if the author has a celebrity cache.

### A user unfollows someone

- Known post IDs are removed immediately.
- Unknown/leftover IDs are removed lazily if they are hydrated.
- Celebrity posts are shared by all followers and are never removed from the shared key on one user's unfollow.

### Cache stampede / thundering herd

The design document recommends singleflight. This implementation reduces the blast radius by:

- serving cold-start pages from a bounded worker pool,
- warming the cache asynchronously after the first miss,
- capping timelines at 800 entries.

A production deployment with many simultaneous cold users should add `golang.org/x/sync/singleflight` around `coldStart` per user, or a short Redis rebuild lock.

---

## 16. Pagination walkthrough

Suppose the cached timeline is:

```text
score 1000  post C
score 900   post B
score 800   post A
```

Request 1: `page_size = 2`, no cursor.

- Fetch top 3.
- Hydrate and return `[C, B]`.
- Cursor = `base64("900:B")`.

Request 2: cursor `900:B`.

- Redis returns scores `<= 900`: `[B, A]`.
- Filter removes `B` because it equals the cursor.
- Return `[A]`.
- No more candidates → `has_more = false`.

If a new post `D` is created between requests:

- its score is `1100`, so it is above the cursor,
- request 2 still starts at `900`,
- no post is skipped or duplicated.

This is the key difference from `LIMIT/OFFSET`, which would shift every row and repeat or skip items.

### Same-timestamp ties

If two posts share a score:

```text
score 900  post B
score 900  post A
```

The feed sorts them by `post_id DESC`. If the cursor is `900:B`, the cursor filter keeps `A` (same score, lower ID). The next request returns `A` exactly once.

---

## 17. Tests

Run:

```bash
cd feed-service
go test ./...
```

Coverage by area:

| Test file | What it verifies |
| --- | --- |
| `services/feed_service_test.go` | cache-hit pagination, end-of-cache behavior, cold start, private filtering, ghost pruning, celebrity merge, Redis fallback, cursor validation, standard fanout, celebrity fanout, private fanout, deletion, follow backfill cap, unfollow purge, event dispatch, invalidation |
| `services/test_helpers_test.go` | thread-safe in-memory cache, Follow/Post mocks, async wait helper |
| `handler/feed_handler_test.go` | proto conversion, required fields, domain-to-gRPC error mapping |
| `repository/timeline_cache_test.go` | no-op cache behavior and Redis key formats |
| `models/events_test.go` | timestamp normalisation, time round-trip, event envelope decoding |

Build:

```bash
go build -o ./tmp/main.exe ./cmd/server
```

Live reload:

```bash
air
```

The root `dev.ps1` and `Procfile` already start `feed-service` alongside the other services.

---

## 18. Integration checklist for the other services

To complete the end-to-end loop:

1. **post-service** should `XADD` a `post.created` event after a successful insert, and a `post.deleted` event after delete/archive.
2. **follow-service** should `XADD` `user.followed` / `user.unfollowed` after the database write.
3. **api-gateway** can add a `getFeed(userId, pageSize, cursor)` query that calls `FeedService.GetFeed`.
4. If author metadata is needed in feed cards, extend `proto/feed.proto` with username/avatar fields and call `user-service` during hydration.

The Feed Service already exposes the RPC and event contracts needed for these steps.

---

## 19. Known limitations / next steps

- **Cold start reads all posts per followed author.** Post Service's `GetAllPosts` has no `limit` parameter. For users following thousands of accounts this is expensive. A `GetRecentPosts(author_id, limit)` or `BatchGetPostsByIds` RPC would be the next optimization.
- **Fanout is synchronous inside the event handler.** A single post with 25,000 active followers performs 25,000 pipelined `ZADD`s. This is acceptable for the current scale; a production system should split fanout into shards and parallel workers.
- **No singleflight yet.** Concurrent cold reads for the same user can duplicate the pull work.
- **No user-service enrichment.** The current `FeedPost` proto only carries post fields and `author_id`.
- **No ranking pipeline.** Ordering is strictly reverse-chronological. The design document's Phase 4 (engagement/affinity ranking) is not implemented.
- **Redis Streams is a local broker substitute.** Replace `internal/events/consumer.go` with a Kafka/RabbitMQ adapter for production if those are available.
- **No authentication in the service.** The API gateway is expected to authenticate the caller and pass a trusted `user_id`.

---

## 20. Quick reference: important constants

| Constant | Value | Location |
| --- | --- | --- |
| `defaultPageSize` | `20` | `services/options.go` |
| `maxPageSize` | `100` | `services/options.go` |
| `timelineMaxSize` default | `800` | `services/options.go` |
| `timelineTTL` default | `720h` | `services/options.go` |
| `activeUserTTL` default | `14d` | `services/options.go` |
| `celebrityThreshold` default | `25000` | `services/options.go` |
| `celebrityRecentLimit` default | `20` | `services/options.go` |
| `followBackfillLimit` | `50` | `services/fanout_service.go` |
| `rpcTimeout` | `3s` | `repository/clients.go` |
| `authorWorkers` | `10` | `repository/clients.go` |
| `hydrationWorkers` | `15` | `repository/clients.go` |
| `timelineChunkSize` | `500` | `repository/timeline_cache.go` |
| Redis Stream reclaim idle | `30s` | `events/consumer.go` |
