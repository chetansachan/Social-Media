# Feed Service: Comprehensive Code & Architecture Documentation

## 1. Overview & System Mission

The **Feed Service** is a high-performance, read-intensive microservice responsible for generating, indexing, and delivering real-time personalized timelines ("Home Feeds") for users across the platform.

### Architectural Challenges & Key Goals
1. **Ultra-Low Read Latency**: P95 feed queries under 50ms using in-memory Redis Sorted Sets (`ZSET`).
2. **Extreme Read/Write Asymmetry**: Feeds are read hundreds of times more frequently than posts are published; the read path is completely decoupled from PostgreSQL database operations.
3. **Write Amplification Mitigation**: Standard users use **Fanout-on-Write (Push)** for instant delivery, whereas high-follower creators use **Fanout-on-Read (Pull)** to eliminate hotkey spikes (Modern **Hybrid Fanout Architecture**).
4. **Resilience & Graceful Degradation**: If Redis is unconfigured or temporarily down, the service falls back seamlessly to dynamic multi-service pull aggregation from `follow-service` and `post-service` without crashing or returning errors to clients.
5. **No N+1 Remote Bottlenecks**: Timeline IDs are hydrated via concurrent worker pools.
6. **Ghost Post Elimination**: Deleted or newly private posts are filtered out during read hydration and asynchronously pruned from follower cache indices.

---

## 2. Directory Structure & Alignment with Post Service

The project strictly adheres to the Clean Architecture / Layered Architecture established in `post-service`:

```
feed-service/
├── cmd/
│   └── server/
│       └── main.go                         # Service bootstrap, gRPC listener, dependency injection & graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go                       # Environment variable loader & typed configuration
│   ├── database/
│   │   └── redis.go                        # Redis client initialization, connection pooling & health checks
│   ├── handler/
│   │   ├── feed_handler.go                 # gRPC transport handler implementing proto.FeedServiceServer
│   │   └── feed_handler_test.go            # Unit tests for gRPC transport layer
│   ├── models/
│   │   ├── errors.go                       # Domain sentinel errors
│   │   ├── events.go                       # Message broker event contracts (PostCreated, UserFollowed, etc.)
│   │   └── feed.go                         # Feed domain models & timeline DTOs
│   ├── repository/
│   │   ├── clients.go                      # gRPC client adapters for FollowService and PostService
│   │   ├── timeline_cache.go               # Redis Sorted Set timeline store with pipelined operations
│   │   ├── timeline_cache_test.go          # Cache and key formatter unit tests
│   │   └── feed_repository.go              # Unified repository interface combining cache and client adapters
│   └── services/
│       ├── fanout_service.go               # Write-path async event processor (push fanout, celebrity routing, backfill)
│       ├── feed_service.go                 # Read-path engine (Redis slicing, celebrity pull-merge, hydration, cursor pagination)
│       └── feed_service_test.go            # Comprehensive service test suite covering all hybrid workflows
├── .air.toml                               # Live reloading configuration for Air
├── .env.example                            # Example environment variables with production defaults
├── .gitignore                              # Git ignore rules for builds, binaries, and environment files
├── go.mod                                  # Go module definition and dependencies
├── go.sum                                  # Checksums for reproducible builds
├── makefile                                # Build commands
├── SYSTEM_DESIGN.md                        # High-level architecture specification document
└── CODE_EXPLANATION.md                     # Complete technical breakdown (this document)
```

---

## 3. Data Structures & Redis Cache Storage Strategy

### Why Store Only IDs?
Storing full post payloads (author name, text, like count, media URLs) in follower feeds causes memory explosion, stale engagement counts, and invalidation nightmares. Instead, the timeline cache stores only **lightweight post references**.

### Redis Sorted Sets (`ZSET`)
Each user's precomputed timeline is stored as a Redis Sorted Set:
* **Key**: `timeline:<user_id>`
* **Score**: 64-bit Timestamp representing post creation time (`float64(CreatedAt.UnixNano())`).
* **Member / Value**: Post ID string (UUID).

#### Key Operations:
1. **Pipelined Batch Push (`PushBatch`)**:
   During post fanout, post IDs are pushed to hundreds of active followers simultaneously using Redis pipelines:
   ```redis
   ZADD timeline:<follower_id> <score> <post_id>
   ZREMRANGEBYRANK timeline:<follower_id> 0 -(maxSize + 1)
   EXPIRE timeline:<follower_id> <ttl>
   ```
2. **Reverse Chronological Slicing (`ZRevRangeByScoreWithScores`)**:
   Feed queries retrieve the latest items using exclusive bounds when a pagination cursor is present:
   ```redis
   ZREVRANGEBYSCORE timeline:<user_id> (<cursor_score> -inf LIMIT 0 <pageSize + 1>
   ```
3. **Celebrity Timeline Cache**:
   Celebrities (> 25,000 followers) do not fan out to followers. Instead, their latest posts are stored in a dedicated key:
   * **Key**: `celebrity:recent:<author_id>`
4. **Active User Tracking**:
   * **Key**: `user:active:<user_id>` (TTL e.g. 14–30 days)
   * Prevents pushing post IDs to dormant accounts, drastically reducing Redis memory usage.

---

## 4. Detailed Component Walkthrough

### 4.1. Configuration (`internal/config/config.go`)
Loads environment variables from `.env` using `godotenv` with production-ready fallbacks:
* `Port`: gRPC listen port (default `50054`).
* `FollowServiceAddr`: gRPC target for Follow Service (`localhost:50053`).
* `PostServiceAddr`: gRPC target for Post Service (`localhost:50052`).
* `RedisAddr`: Redis address (`localhost:6379`).
* `RedisPassword` & `RedisDB`: Authentication and database index.
* `TimelineMaxSize`: Max items retained in each user's timeline (default `800`).
* `TimelineTTLHours`: Timeline cache TTL (default `720` hours / 30 days).
* `CelebrityThreshold`: Follower cutoff for celebrity pull classification (default `25,000`).
* `InactiveThresholdDays`: Inactivity threshold before skipping fanout writes (default `14`).

### 4.2. Database & Cache Engine (`internal/database/redis.go`)
Initializes the `go-redis/v9` client with tuned connection pooling:
* `PoolSize`: 50 connections.
* `MinIdleConns`: 10 connections.
* `DialTimeout`, `ReadTimeout`, `WriteTimeout`: 2–3s bounds.
* Verifies connection with `Ping`. If Redis is unavailable, logs a warning and permits the service to start in pull-model fallback mode.

### 4.3. Models & Events (`internal/models/`)
* `errors.go`: Typed sentinel errors (`ErrInvalidInput`, `ErrInvalidCursor`, `ErrUserNotFound`, `ErrCacheUnavailable`).
* `events.go`: Event payloads for asynchronous message buses:
  * `PostCreatedEvent`: Published when a post is created.
  * `PostDeletedEvent`: Published when a post is deleted.
  * `UserFollowedEvent`: Follower and following IDs.
  * `UserUnfollowedEvent`: Follower and following IDs.
* `feed.go`:
  * `FeedPost`: Hydrated feed card containing post details, author ID, like count, and timestamps.
  * `GetFeedQuery`: Input DTO containing `UserID`, `PageSize`, and `Cursor`.
  * `FeedResult`: Response DTO containing `Posts`, `NextCursor`, and `HasMore`.
  * `TimelineItem`: Lightweight cache element containing `PostID` and `Score`.

### 4.4. Repository & Client Adapters (`internal/repository/`)
* **`timeline_cache.go` (`TimelineCache`)**:
  * Implements `PushToTimeline`, `PushBatch`, `GetTimeline`, `RemovePost`, `InvalidateTimeline`, `BackfillTimeline`, `StoreCelebrityPost`, and `IsUserActive`.
  * Includes `noopTimelineCache` for graceful fallback when Redis is offline.
* **`clients.go` (`FollowClient`, `PostClient`)**:
  * Wraps gRPC stubs generated from `proto/follow.proto` and `proto/post.proto`.
  * `BatchGetPosts`: Concurrently hydrates up to 15 posts in parallel using a bounded worker pool (`chan struct{}`), eliminating the N+1 network call problem while preserving the chronological order requested.
* **`feed_repository.go` (`FeedRepository`)**:
  * Unified facade providing access to `TimelineCache()`, `FollowClient()`, and `PostClient()`.

### 4.5. Services & Business Logic (`internal/services/`)

#### The Read Path (`feed_service.go`)
```
Client Request ──▶ Validate UserID & Parse Cursor
                          │
                          ▼
            Query Redis: `timeline:<user_id>`
           /                                 \
  (Cache Miss or Empty)               (Cache Hit)
         /                                     \
Fetch Followed Accounts               Fetch Followed Celebrities
Fetch Posts Directly from Post-Svc    Pull Celebrity Posts & Merge
Sort by Timestamp                     Batch Hydrate from Post-Svc
Async Backfill into Redis             Filter Ghost Posts & Prune Cache
Paginate & Return                     Paginate & Return
```
1. **Cursor Validation**: Parses opaque token `<unix_nano>:<post_id>` with exact nanosecond precision.
2. **Timeline Slicing**: Retrieves the top `pageSize + 1` post IDs from Redis with score `< cursor_score`.
3. **Celebrity Merging**: For followed users whose follower count exceeds `CelebrityThreshold`, recent posts are pulled on-the-fly and merged in memory with the push items.
4. **Batch Hydration**: Calls `PostClient.BatchGetPosts` in parallel.
5. **Ghost Post Pruning**: Identifies missing or deleted posts, filters them out, and launches background goroutines to prune those dead IDs from the user's Redis `ZSET`.
6. **Cold-Start Fallback**: If the user's cache is empty or expired, the service queries `follow-service`, fetches recent posts for all followed users, sorts them, and asynchronously backfills Redis so subsequent scrolls hit the cache instantly.

#### The Write Path (`fanout_service.go`)
1. **`FanoutPost`**:
   * Pushes post to author's own timeline so the author sees their own publication immediately.
   * Checks if author follower count $\ge 25,000$:
     * **Celebrity**: Stores in `celebrity:recent:<author_id>` (Fanout-on-Read).
     * **Standard**: Fetches followers from `follow-service`, filters active followers (logged in within last 14 days), and pushes to their timelines via Redis pipeline (Fanout-on-Write).
2. **`HandlePostDeleted`**:
   * Eagerly invalidates post ID from author timeline. Lazy read pruning cleans followers' caches.
3. **`HandleUserFollowed`**:
   * Backfills up to 50 recent posts from the followed user into the follower's timeline cache.
4. **`HandleUserUnfollowed`**:
   * Purges the unfollowed user's posts from the follower's timeline cache.

### 4.6. Transport Handler (`internal/handler/feed_handler.go`)
Implements gRPC `proto.FeedServiceServer`:
* `GetFeed(ctx, *proto.GetFeedRequest) (*proto.GetFeedResponse, error)`
* `InvalidateUserFeed(ctx, *proto.InvalidateUserFeedRequest) (*proto.InvalidateUserFeedResponse, error)`
* Maps domain errors to standard gRPC status codes:
  * `ErrInvalidInput`, `ErrInvalidCursor` $\rightarrow$ `codes.InvalidArgument`
  * `ErrUserNotFound` $\rightarrow$ `codes.NotFound`
  * Unexpected system errors $\rightarrow$ `codes.Internal`

### 4.7. Service Entrypoint (`cmd/server/main.go`)
* Loads configuration.
* Connects to Redis (logs warning on failure; falls back gracefully).
* Establishes gRPC client connections to `FollowService` and `PostService`.
* Wires up dependencies via Constructor Injection.
* Registers `FeedServiceServer` on standard gRPC server.
* Listens on TCP port and handles `SIGINT`/`SIGTERM` for graceful server shutdown.

---

## 5. Pagination & Cursor Mechanics

Offset pagination (`OFFSET 20 LIMIT 20`) causes severe feed drift in real-time feeds when new items are added while a user is reading.

Feed Service utilizes **Cursor-Based Pagination**:
* **Token Structure**: `base64URL("<unix_nanoseconds>:<post_id>")`
* **Redis Query**:
  ```go
  opt := &redis.ZRangeBy{
      Min:    "-inf",
      Max:    fmt.Sprintf("(%f", cursorScore), // exclusive upper bound
      Offset: 0,
      Count:  limit,
  }
  ```
* **Deduplication & Deterministic Tie-Breaking**:
  If two posts have the exact same millisecond timestamp, the tie is broken deterministically by `post_id`:
  ```go
  postNano < cursorNano || (postNano == cursorNano && post.ID < cursorID)
  ```

---

## 6. How to Run & Verify

### 1. Run Unit Tests
Execute the full test suite across all packages:
```bash
cd feed-service
go test -v ./...
```
Expected output:
```
=== RUN   TestFeedHandler_GetFeed_Success
--- PASS: TestFeedHandler_GetFeed_Success (0.00s)
=== RUN   TestFanoutAndCacheRead
--- PASS: TestFanoutAndCacheRead (0.00s)
=== RUN   TestCelebrityHybridFanout
--- PASS: TestCelebrityHybridFanout (0.00s)
=== RUN   TestFollowBackfillAndUnfollow
--- PASS: TestFollowBackfillAndUnfollow (0.00s)
=== RUN   TestColdStartFallback_SortFilterAndPaginate
--- PASS: TestColdStartFallback_SortFilterAndPaginate (0.00s)
PASS
```

### 2. Build the Service Binary
```bash
go build -o ./tmp/main.exe ./cmd/server
```

### 3. Run with Air (Live Reload)
From `feed-service` directory:
```bash
air
```

### 4. Run the Full Microservices Suite
From the repository root:
```powershell
.\dev.ps1
```
This launches `user-service`, `post-service`, `follow-service`, `feed-service`, and `api-gateway` concurrently.
