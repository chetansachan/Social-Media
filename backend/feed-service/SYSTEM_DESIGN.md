# System Design: Feed Service (Timeline & Activity Stream)

## 1. Executive Summary & Purpose

The **Feed Service** is responsible for delivering a personalized, fast, and relevant stream of content (the "Newsfeed" or "Home Timeline") to users. When a user opens the application, they expect to immediately see the latest posts published by people, creators, and accounts they follow, ordered chronologically or algorithmically, with smooth infinite scrolling.

In a microservices architecture, the Feed Service is one of the most read-intensive and performance-critical services. It does not own the user profiles (owned by `user-service`), the raw posts (owned by `post-service`), or the follow graph (owned by `follow-service`). Instead, it acts as the **aggregator, indexer, and real-time distributor** of post references.

---

## 2. Requirements & Design Targets

### Functional Requirements
1. **View Feed**: A user can fetch a chronological list of posts created by accounts they follow.
2. **Infinite Scroll (Pagination)**: The feed supports seamless pagination using cursor-based navigation without duplicate posts or skipped content.
3. **Real-time Ingestion**: When an account creates a new post, followers should see it in their feed with minimal latency (< 2 seconds).
4. **Post Updates & Deletions**: Deleted posts or updated visibility (e.g. public to private) must be reflected without stale "ghost" items appearing in user feeds.
5. **Relationship Changes**: When a user follows or unfollows another user, the feed must adapt gracefully.

### Non-Functional Requirements
1. **Ultra-Low Read Latency**: Target **p95 < 50ms**, **p99 < 150ms** for feed retrieval.
2. **High Read-to-Write Ratio**: Social media feeds typically experience an extreme asymmetry (e.g., 100:1 or 1000:1 read vs write ratio). The read path must be heavily decoupled from database writes.
3. **Eventual Consistency**: It is acceptable if a new post takes 1 to 2 seconds to appear in all followers' feeds. Strong consistency across millions of followers simultaneously is neither necessary nor scalable.
4. **Resilience & Availability**: If the feed cache or a background worker fails, the system must degrade gracefully (fallback to database querying) rather than failing outright.

---

## 3. High-Level System Architecture

```
                               ┌─────────────────────────┐
                               │       Client Apps       │
                               │    (Web / iOS / Android)│
                               └────────────┬────────────┘
                                            │
                                            ▼
                               ┌─────────────────────────┐
                               │       API Gateway       │
                               │    (GraphQL / REST)     │
                               └────────────┬────────────┘
                                            │ (gRPC)
                                            ▼
 ┌────────────────────────────────────────────────────────────────────────────────────────┐
 │                                   FEED SERVICE                                         │
 │                                                                                        │
 │  ┌─────────────────────────┐                          ┌─────────────────────────────┐  │
 │  │    Feed Query API       │                          │     Fanout Worker Pool      │  │
 │  │ (gRPC Read Handler)     │                          │   (Async Event Consumers)   │  │
 │  └───────────┬─────────────┘                          └──────────────┬──────────────┘  │
 │              │                                                       │                 │
 │              ▼                                                       ▼                 │
 │  ┌──────────────────────────────────────────────────────────────────────────────────┐  │
 │  │                              Feed Cache Engine                                   │  │
 │  │           (Redis Cluster: In-Memory Sorted Sets for User Timelines)              │  │
 │  └──────────────────────────────────────────────────────────────────────────────────┘  │
 └──────────────┬───────────────────────────┬───────────────────────────┬─────────────────┘
                │                           │                           │
                ▼                           ▼                           ▼
     ┌─────────────────────┐     ┌─────────────────────┐     ┌─────────────────────┐
     │   Follow Service    │     │    Post Service     │     │    User Service     │
     │  (Social Graph)     │     │ (Post Source Truth) │     │ (Author Metadata)   │
     └─────────────────────┘     └─────────────────────┘     └─────────────────────┘
                ▲
                │ Events (PostCreated, PostDeleted, UserFollowed, UserUnfollowed)
     ┌─────────────────────────────────────────────────────────────────────────────────────┐
     │                     Message Broker / Event Bus (Kafka / RabbitMQ)                   │
     └─────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Internal Components of the Feed Service

To keep responsibilities clean and prevent bottlenecks, the Feed Service is divided into distinct internal sub-modules:

### 1. Feed Retrieval & API Handler
* Serves client read requests via gRPC.
* Validates user authentication and extracts pagination cursors.
* Queries the **Feed Cache Engine** for a slice of Post IDs belonging to the user's timeline.
* Dispatches concurrent batch hydration requests to `post-service` and `user-service`.
* Formats and returns the hydrated timeline.

### 2. Fanout Worker Service (Write Path Processor)
* Subscribes to events from the Message Broker (e.g. `PostCreated`, `PostDeleted`).
* Looks up the author's followers via `follow-service`.
* Distributes post references into the timeline caches of all eligible active followers.
* Applies celebrity/hotkey routing rules (see Hybrid Model in Section 5).

### 3. Cache Management Service (Timeline Store)
* Encapsulates all operations against the fast in-memory store (Redis).
* Manages user timeline data structures (storing sorted lists of `PostID` and `Timestamp`).
* Enforces cache sizing policies (e.g., capping timelines at 800 items per user).
* Handles timeline cache expiration (TTL) for inactive accounts.

### 4. Hydration & Aggregation Engine
* Transforms a raw list of post IDs (`[post_1, post_2, post_3]`) into rich feed objects.
* Executes multi-get requests (`BatchGetPosts`) against `post-service` to avoid N+1 remote calls.
* Enriches posts with author profile information (username, avatar, display name) from `user-service`.
* Enriches posts with the viewer's personal context (e.g., `has_liked`, `is_bookmarked`).

### 5. Relationship Event Processor
* Listens to `UserFollowed` and `UserUnfollowed` events.
* When User A follows User B: backfills User B’s recent posts into User A's timeline cache.
* When User A unfollows User B: purges User B’s posts from User A's timeline cache.

---

## 5. Architectural Paradigms: Fanout Models

The primary architectural challenge of any feed system is determining **when** to compute the feed: when a post is created (write time), or when a user views their timeline (read time).

### Model A: Fanout-on-Read (Pull Model)

In the pull model, no feeds are precomputed when an author creates a post.

```
Author writes post ───────────▶ Saved to Post DB (Nothing else happens)

Viewer requests feed ─────────▶ 1. Fetch all authors viewer follows (from Follow Service)
                                2. Query posts DB for latest posts from ALL those authors
                                3. Merge, sort in memory, and paginate
                                4. Return top 20 posts to viewer
```

* **Pros:**
  * Post creation is extremely fast ($O(1)$ write).
  * Inactive users consume zero storage or compute resources.
  * No write-amplification problem when creators with millions of followers publish.
* **Cons:**
  * **Painfully slow read performance**: If a user follows 2,000 accounts, the system must query 2,000 authors' posts, merge them, and sort them on every single page scroll.
  * Heavy database CPU utilization under high read volume.
  * High risk of timeouts and cascading failures during traffic spikes.

---

### Model B: Fanout-on-Write (Push Model)

In the push model, every user has an individual pre-computed inbox (timeline). When an author writes a post, the system immediately pushes that post ID into every follower's timeline cache.

```
Author writes post ───────────▶ 1. Saved to Post DB
                                2. Event emitted to Message Broker
                                3. Fanout Workers fetch all 500 followers
                                4. Post ID pushed to all 500 followers' timeline caches

Viewer requests feed ─────────▶ 1. Read top 20 Post IDs from viewer's precomputed cache
                                2. Hydrate post details
                                3. Return immediately (Ultra-fast O(1) read)
```

* **Pros:**
  * **Blazing fast reads**: The feed is pre-assembled. Fetching 20 items from an in-memory sorted set takes < 2ms.
  * Read load does not scale with the number of accounts followed.
* **Cons:**
  * **Write Amplification**: If an account with 20 million followers posts, one single write turns into 20 million cache update operations.
  * Wasted resources: Pushing posts to millions of users who haven't logged in for months wastes RAM and CPU.

---

### Model C: The Hybrid Fanout Model (Recommended Industry Standard)

To get the best of both worlds, modern social networks (Twitter/X, Instagram) utilize a **Hybrid Fanout Model** based on user follower counts and activity status:

```
                            ┌───────────────────────────────────┐
                            │        Post Created Event         │
                            └─────────────────┬─────────────────┘
                                              │
                                              ▼
                             Is author follower count > 25,000?
                                            /   \
                                      YES  /     \  NO
                                          /       \
                                         ▼         ▼
                           ┌─────────────────┐ ┌─────────────────────────┐
                           │   DO NOT PUSH   │ │       PUSH FANOUT       │
                           │  (Celebrity)    │ │   Push to all active    │
                           │ Store in author │ │  followers' timeline    │
                           │   recent list   │ │         caches          │
                           └─────────────────┘ └─────────────────────────┘
                                       \                 /
                                        \               /
                                         ▼             ▼
                               ┌─────────────────────────────────────────┐
                               │         When Follower Reads Feed:       │
                               │ 1. Read precomputed cache (push items)  │
                               │ 2. Fetch latest posts of followed       │
                               │    celebrities on the fly (pull items)  │
                               │ 3. Merge & sort in memory               │
                               │ 4. Hydrate & return                     │
                               └─────────────────────────────────────────┘
```

1. **Standard Users (< 25,000 followers)**:
   * Processed using **Fanout-on-Write (Push)**.
   * Fast, low overhead, and keeps 99% of user feeds precomputed.
2. **Celebrity / High-Follower Users (> 25,000 followers)**:
   * Processed using **Fanout-on-Read (Pull)**.
   * Eliminates the catastrophic 10-million write amplification on post creation.
3. **Active vs. Inactive Filter**:
   * Posts are only pushed to followers who have logged in or interacted with the platform within the last 14–30 days.

---

## 6. End-to-End Workflows

### 1. The Post Creation & Fanout Workflow (Write Path)

```
[User App] ──1. CreatePost──▶ [Post Service] ──2. Persist to Postgres──┐
                                                                       │
                                 ┌──4. Publish PostCreatedEvent ◄──────┘
                                 │
                                 ▼
                     [Event Bus / Message Queue]
                                 │
                                 ▼ (Consumed by)
                     [Feed Fanout Worker Pool]
                                 │
                5. Check Author Follower Count & Tier
                                 │
           ┌─────────────────────┴─────────────────────┐
           ▼ (Author is Standard)                      ▼ (Author is Celebrity)
 6a. Call Follow Service:                   6b. Append PostID to Author's
     GetFollowers(author_id)                    Public Post Timeline Cache
 7a. Filter Active Followers                    (Skip pushing to individual
 8a. Batch Pipeline update to Redis:            followers' inboxes)
     ZADD timeline:<follower_id> timestamp post_id
 9a. Trim timelines to max limit (e.g. 800 items)
```

### 2. The Feed Retrieval Workflow (Read Path)

```
[User App] ──1. GetFeed(page_size, cursor)──▶ [API Gateway] ──2. gRPC──▶ [Feed Service]
                                                                               │
 ┌─────────────────────────────────────────────────────────────────────────────┘
 │
 ▼
3. Query Redis Cache:
   Fetch post IDs from `timeline:<user_id>` using ZREVRANGEBYSCORE with cursor
4. Pull Celebrity Posts (if hybrid model enabled):
   Identify followed celebrities from `follow-service`, fetch their latest posts
5. Merge & Sort:
   Combine push-cache IDs and pulled celebrity IDs; sort descending by timestamp
6. Batch Hydration:
   Call `PostService.BatchGetPosts(post_ids)` to retrieve content, media, like counts
7. Metadata & Privacy Check:
   Filter out any deleted or private posts returned from hydration
8. Enrich Author Information:
   Call `UserService.BatchGetUsers(author_ids)` for avatar, name, handle
9. Return Feed Response:
   Assemble complete feed cards with next-page cursor and return to client
```

### 3. The Unfollow / Follow Relationship Change Workflow

* **When User A follows User B:**
  * An event `UserFollowed(follower: A, following: B)` is emitted.
  * Feed worker fetches User B's latest 50 posts from `post-service`.
  * Injects those 50 posts into User A's Redis sorted set `timeline:A` with their original timestamps.
* **When User A unfollows User B:**
  * An event `UserUnfollowed(follower: A, following: B)` is emitted.
  * **Optimistic / Lazy Clean**: Rather than scanning the entire cache to purge posts immediately, the feed service can filter out User B's posts during read hydration, or background workers can asynchronously remove them.

---

## 7. Storage & Data Structures Strategy

### Why Storing Full Posts in Feed Cache is an Anti-Pattern
Never duplicate full post data (text, media URLs, author info, like counts) inside user feeds:
* **Memory Explosion**: Storing full objects for 500 followers per post multiplies memory by 500x.
* **Stale Counts**: Like counts and comment counts change constantly; cached copies become stale instantly.
* **Invalidation Nightmare**: If an author edits their post text, thousands of cached timelines would need updating.

### The Correct Approach: Lightweight ID References in Redis

Each user timeline is represented as a **Redis Sorted Set (`ZSET`)**:
* **Key**: `timeline:<user_id>`
* **Score**: 64-bit Timestamp (epoch milliseconds or nanoseconds)
* **Member / Value**: `post_id` (UUID string)

#### Key Operational Advantages:
1. **Natural Chronological Ordering**: Redis sorted sets automatically keep members ordered by score.
2. **$O(\log N + M)$ Slicing**: Fetching a page of 20 posts (`ZREVRANGEBYSCORE`) takes fractions of a millisecond.
3. **Automatic Deduplication**: Because members are unique, the same post can never be inserted twice in a user's timeline.
4. **Memory Optimization**: Storing 800 UUIDs per active user requires only ~30–40 KB of RAM. One gigabyte of Redis memory can hold the active timelines of ~25,000 to 30,000 active users.

---

## 8. Critical Engineering Problems & Failure Modes

Building a reliable Feed Service involves overcoming several well-known distributed systems challenges:

---

### Problem 1: The "Celebrity" / Hotkey Write Amplification Problem
* **The Issue:** When an account with 15 million followers publishes a post, a pure push architecture must perform 15 million write operations into 15 million different Redis keys. This creates message queue backlog, CPU spikes, and severe lag. Followers might not see the post until 10 minutes later.
* **The Solution:**
  1. Classify accounts with high follower counts as "celebrities".
  2. Stop pushing their posts to followers' inboxes.
  3. When followers open their feed, dynamically fetch the latest posts of the celebrities they follow and merge them into the feed in memory.

---

### Problem 2: Feed Drift & Broken Pagination with Offset
* **The Issue:** Traditional SQL `LIMIT 20 OFFSET 20` pagination breaks on real-time feeds:
  * User reads posts 1 to 20.
  * While reading, 5 new posts are published.
  * User scrolls down; `OFFSET 20` now skips 5 posts and repeats 5 posts they have already seen.
* **The Solution: Cursor-Based Pagination:**
  * Use a cursor composed of `(timestamp, post_id)` corresponding to the last item on the previous page.
  * Query Redis using `ZREVRANGEBYSCORE timeline:<user_id> (cursor_timestamp -inf LIMIT 0 20`.
  * New posts published at the top do not shift the index of older posts below the cursor.

---

### Problem 3: The "Ghost Post" Problem (Deletions & Privacy Toggles)
* **The Issue:** If an author deletes a post or marks it private, their post ID might still linger in thousands of followers' Redis timelines.
* **The Solution:**
  * **Two-Layer Strategy:**
    1. **Eager Invalidation for Author Timeline**: Immediately delete from author profile.
    2. **Lazy Pruning at Read Time (Hydration Filter)**: When the Feed Service calls `PostService.BatchGetPosts([ids])`, `post-service` returns `nil` or omits deleted/inaccessible posts. The Feed Service drops these missing items from the response and schedules a background task to clean that ID out of the user's Redis `ZSET`.

---

### Problem 4: Dormant & Inactive Users Wasting Memory
* **The Issue:** Over months, millions of registered users stop opening the app. Continuing to push posts into their feeds wastes expensive RAM and processing power.
* **The Solution:**
  1. Maintain an `active_users` set or check `last_login_at`.
  2. If a follower has been inactive for > 14 days, **skip** pushing new post IDs to their timeline.
  3. Configure a Redis TTL (e.g., 30 days) on timeline keys so abandoned timelines expire automatically.
  4. **Cold Start Rebuild**: When an inactive user logs back in, trigger an on-demand feed reconstruction that pulls recent posts from their followed accounts and populates their cache.

---

### Problem 5: The N+1 Remote Call Problem
* **The Issue:** After obtaining 20 post IDs from Redis, making 20 individual gRPC calls to `post-service` introduces network round-trip serialization overhead that destroys read latency.
* **The Solution:**
  * Implement batch gRPC endpoints: `rpc BatchGetPostsByIds(BatchGetPostsRequest) returns (BatchGetPostsResponse)`.
  * Fetch all 20 posts in a single network round-trip.
  * Use internal goroutines/concurrency to fetch author profile data and viewer like states in parallel.

---

### Problem 6: Thundering Herd & Cache Stampede
* **The Issue:** When a popular user's timeline cache expires or is invalidated, hundreds of concurrent requests for that feed might hit the database simultaneously, causing database crashes.
* **The Solution:**
  * Implement **Singleflight** (request deduplication) or distributed mutex locks.
  * If the cache is a miss, exactly one request queries the database and rebuilds the cache, while duplicate concurrent requests wait for that single result.

---

## 9. Conceptual gRPC Interface & Event Contracts

### 1. Feed Service RPC Definitions

```
Service: FeedService
-----------------------------------------------------------------------------
Method: GetFeed
  Request:
    - user_id: string          (The user requesting their timeline)
    - page_size: int32         (Number of items requested, e.g., 20)
    - cursor: string           (Opaque pagination token: timestamp + post_id)
  Response:
    - posts: list of FeedPost  (Hydrated posts with author info and viewer state)
    - next_cursor: string      (Token to pass for the next page)
    - has_more: bool           (Flag indicating if further pages exist)

Method: InvalidateUserFeed
  Request:
    - user_id: string
  Response:
    - success: bool
```

### 2. Message Bus Event Contracts

```
Event: PostCreatedEvent
  Payload:
    - post_id: string
    - author_id: string
    - created_at: int64 (Unix Timestamp)
    - is_public: bool

Event: PostDeletedEvent
  Payload:
    - post_id: string
    - author_id: string

Event: UserFollowedEvent
  Payload:
    - follower_id: string
    - following_id: string

Event: UserUnfollowedEvent
  Payload:
    - follower_id: string
    - following_id: string
```

---

## 10. Summary: The Step-by-Step Evolution Path

When building `feed-service` for this social media project, follow this natural progression:

| Phase | Architecture Level | Capabilities |
| :--- | :--- | :--- |
| **Phase 1 (MVP)** | Simple Pull Model | Feed Service queries `follow-service` for followed user IDs, queries `post-service` for their latest posts, sorts in memory, and returns. Fast to build, good for testing. |
| **Phase 2 (Scalable)** | Push Model + Redis | Add Message Queue events for `PostCreated`. Fanout workers push post IDs into Redis `ZSET` timelines. Feed reads become instant $O(1)$ operations. |
| **Phase 3 (Production)** | Hybrid Model + Optimization | Introduce celebrity threshold, active-user filtering, batch hydration endpoints, cursor pagination, and cold-start rebuild for dormant accounts. |
| **Phase 4 (Advanced)** | Algorithmic Ranking | Add a scoring and ranking pipeline considering engagement, recency, and user affinity rather than strictly chronological ordering. |
