# SocialFlow Engineering Master Guide: System Optimization, High-Scale Architecture (Millions of Users), and Service Testing Blueprint

---

## Table of Contents

1. [Executive Audit: Current State vs. Production-Ready Platform](#1-executive-audit-current-state-vs-production-ready-platform)
2. [What Else is Left to Make (Service-by-Service Gap Analysis)](#2-what-else-is-left-to-make-service-by-service-gap-analysis)
   - [2.1 API Gateway Wiring & GraphQL Capabilities](#21-api-gateway-wiring--graphql-capabilities)
   - [2.2 Event Publishing & The Transactional Outbox Pattern](#22-event-publishing--the-transactional-outbox-pattern)
   - [2.3 Asynchronous Media Upload Pipeline](#23-asynchronous-media-upload-pipeline)
   - [2.4 Missing Core Services & Features](#24-missing-core-services--features)
   - [2.5 Authentication, Token Lifecycle & Session Revocation](#25-authentication-token-lifecycle--session-revocation)
3. [Architecting for Millions of Users (High Concurrency & Scale-Out)](#3-architecting-for-millions-of-users-high-concurrency--scale-out)
   - [3.1 Target Scale & Traffic Math (50,000+ QPS)](#31-target-scale--traffic-math-50000-qps)
   - [3.2 Edge, CDN & Layer-7 Load Balancing](#32-edge-cdn--layer-7-load-balancing)
   - [3.3 Database Scaling: Read/Write Splitting, Partitioning & Citus Sharding](#33-database-scaling-readwrite-splitting-partitioning--citus-sharding)
   - [3.4 Caching Architecture: Singleflight, Thundering Herds & Redis Cluster](#34-caching-architecture-singleflight-thundering-herds--redis-cluster)
   - [3.5 Event Bus Evolution: Redis Streams to Apache Kafka / Redpanda](#35-event-bus-evolution-redis-streams-to-apache-kafka--redpanda)
   - [3.6 Millions of WebSocket Connections (Gateway Pod Sharding)](#36-millions-of-websocket-connections-gateway-pod-sharding)
4. [Performance Optimizations (DB, Memory, Go Runtime)](#4-performance-optimizations-db-memory-go-runtime)
   - [4.1 PostgreSQL Engine Tuning & Indexing Strategies](#41-postgresql-engine-tuning--indexing-strategies)
   - [4.2 Go Runtime, Garbage Collector & Memory Tuning](#42-go-runtime-garbage-collector--memory-tuning)
   - [4.3 gRPC & Protocol Buffer Throughput Tuning](#43-grpc--protocol-buffer-throughput-tuning)
   - [4.4 GraphQL Dataloader (Eliminating N+1 Queries)](#44-graphql-dataloader-eliminating-n1-queries)
5. [Security, Resilience & Fault Tolerance](#5-security-resilience--fault-tolerance)
   - [5.1 Circuit Breaking, Timeouts & Deadlines](#51-circuit-breaking-timeouts--deadlines)
   - [5.2 Distributed Rate Limiting & Bot Defense](#52-distributed-rate-limiting--bot-defense)
   - [5.3 Graceful Shutdown & Zero-Downtime Deployments](#53-graceful-shutdown--zero-downtime-deployments)
6. [Observability, Tracing & Metrics](#6-observability-tracing--metrics)
   - [6.1 OpenTelemetry Distributed Tracing (Correlation IDs)](#61-opentelemetry-distributed-tracing-correlation-ids)
   - [6.2 Prometheus Metrics Blueprint](#62-prometheus-metrics-blueprint)
   - [6.3 Structured Logging with `log/slog`](#63-structured-logging-with-logslog)
   - [6.4 Liveness & Readiness Probes](#64-liveness--readiness-probes)
7. [Comprehensive Testing Blueprint & Exact Test Cases for Every Service](#7-comprehensive-testing-blueprint--exact-test-cases-for-every-service)
   - [7.1 Testing Strategy Matrix (Unit, Integration, E2E, Load)](#71-testing-strategy-matrix-unit-integration-e2e-load)
   - [7.2 Service 1: `user-service` Test Cases](#72-service-1-user-service-test-cases)
   - [7.3 Service 2: `post-service` Test Cases](#73-service-2-post-service-test-cases)
   - [7.4 Service 3: `follow-service` Test Cases](#74-service-3-follow-service-test-cases)
   - [7.5 Service 4: `feed-service` Test Cases](#75-service-4-feed-service-test-cases)
   - [7.6 Service 5: `notification-service` Test Cases](#76-service-5-notification-service-test-cases)
   - [7.7 Service 6: `api-gateway` Test Cases](#77-service-6-api-gateway-test-cases)
   - [7.8 End-to-End & Concurrency Stress Test Cases](#78-end-to-end--concurrency-stress-test-cases)
   - [7.9 k6 Load Testing Script Templates](#79-k6-load-testing-script-templates)
8. [Phased Implementation Roadmap & Prioritized Execution Checklist](#8-phased-implementation-roadmap--prioritized-execution-checklist)

---

## 1. Executive Audit: Current State vs. Production-Ready Platform

SocialFlow is architected as an asynchronous, event-driven microservices platform written in Go, exposing a GraphQL API Gateway and communicating internally via gRPC and Redis Streams. 

### Architecture Inventory

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                                   API GATEWAY                                          │
│                           (GraphQL Handler via gqlgen)                                 │
└────────────┬──────────────────────────┬───────────────────────────┬────────────────────┘
             │ gRPC                     │ gRPC                      │ gRPC
             ▼                          ▼                           ▼
  ┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
  │    user-service     │    │    post-service     │    │   follow-service    │
  │     (Port 50051)    │    │     (Port 50052)    │    │     (Port 50053)    │
  │     PostgreSQL      │    │     PostgreSQL      │    │     PostgreSQL      │
  └─────────────────────┘    └──────────┬──────────┘    └──────────┬──────────┘
                                        │                          │
                                        │ (Events: Missing)        │ (Events: Missing)
                                        ▼                          ▼
                             ┌────────────────────────────────────────┐
                             │       Redis Streams (social:events)    │
                             └──────────┬──────────────────┬──────────┘
                                        │                  │
                                        ▼                  ▼
                             ┌─────────────────────┐    ┌─────────────────────┐
                             │    feed-service     │    │notification-service │
                             │     (Port 50054)    │    │(Port 50055 & 8085)  │
                             │  Redis Timeline ZSet│    │PostgreSQL + Redis WS│
                             └─────────────────────┘    └─────────────────────┘
```

### The Critical Gaps Identified in Current State

| Component / Layer | Current Status | Production Gap | Impact |
| :--- | :--- | :--- | :--- |
| **API Gateway** | GraphQL routes only for User, Post, Follow | `feed-service` and `notification-service` clients are not wired into `server.go` or `schema.graphqls`. No GraphQL subscriptions for real-time alerts. | Users cannot fetch their feed or view/manage notifications via the gateway. |
| **Event Publishing** | Ingestion consumers exist in `feed-service` and `notification-service` | `post-service` and `follow-service` **do not publish** `PostCreated`, `PostLiked`, or `UserFollowed` events to Redis Streams. | Feeds and notifications remain static and never receive domain events automatically. |
| **Media Pipeline** | `post-service` accepts raw base64 and uploads synchronously to Cloudinary | Upload blocks the HTTP/gRPC request thread; large payload crashes memory under high QPS. | Severe latency spikes ($> 2\text{s}$ per post), request timeouts, memory exhaustion. |
| **Auth & Security** | JWT generation exists in `user-service` | No JWT validation middleware in `api-gateway`; user claims are not extracted or passed downstream via gRPC metadata context. | Any caller can impersonate any `author_id` or `user_id` without authentication. |
| **Data Consistency** | Direct DB writes without transactional outbox | No guaranteed atomicity between SQL transactions and Redis Streams event emission. | Dual-write failures: DB writes succeed while message queue publish fails, leaving data out of sync. |
| **Test Coverage** | Tests exist in `feed-service` and `notification-service` | Zero unit or integration tests in `user-service`, `post-service`, `follow-service`, or `api-gateway`. | High regression risk during scaling or refactoring. |

---

## 2. What Else is Left to Make (Service-by-Service Gap Analysis)

### 2.1 API Gateway Wiring & GraphQL Capabilities

The API Gateway is the single public entrypoint for client applications (Web, iOS, Android). Currently, it only wires `UserClient`, `PostClient`, and `FollowClient`.

#### What needs to be built:
1. **gRPC Client Connections**:
   * Add connection to `feed-service` (`localhost:50054` or `FEED_SERVICE_ADDR`).
   * Add connection to `notification-service` (`localhost:50055` or `NOTIFICATION_SERVICE_ADDR`).
2. **GraphQL Schema Expansion (`schema.graphqls`)**:
   ```graphql
   # Feed Types & Queries
   type FeedPost {
       id: ID!
       authorId: String!
       content: String!
       mediaUrl: String!
       isPublic: Boolean!
       likeCount: Int!
       hasLiked: Boolean!
       createdAt: String!
       author: User!
   }

   type FeedResponse {
       posts: [FeedPost!]!
       nextCursor: String
       hasMore: Boolean!
   }

   # Notification Types & Queries
   enum NotificationType {
       FOLLOW
       POST_LIKE
       COMMENT
       COMMENT_REPLY
       MENTION
       CREATOR_POST
       SYSTEM_ALERT
   }

   type NotificationActor {
       id: ID!
       username: String!
       displayName: String!
       avatarUrl: String!
   }

   type NotificationItem {
       id: ID!
       type: NotificationType!
       entityId: String!
       entityType: String!
       actors: [NotificationActor!]!
       totalActorsCount: Int!
       title: String!
       contentPreview: String
       actionUrl: String!
       isRead: Boolean!
       createdAt: String!
   }

   type NotificationResponse {
       notifications: [NotificationItem!]!
       nextCursor: String
       hasMore: Boolean!
       unreadCount: Int!
   }

   extend type Query {
       getFeed(limit: Int, cursor: String): FeedResponse!
       getNotifications(limit: Int, cursor: String, filter: NotificationType): NotificationResponse!
       getUnreadNotificationCount: Int!
       getNotificationSettings: NotificationSettings!
   }

   extend type Mutation {
       markNotificationAsRead(notificationIds: [ID!]!): Boolean!
       markAllNotificationsAsRead: Boolean!
       updateNotificationSettings(input: UpdateNotificationSettingsInput!): NotificationSettings!
       registerDeviceToken(token: String!, platform: String!): Boolean!
   }

   extend type Subscription {
       notificationReceived: NotificationItem!
   }
   ```
3. **GraphQL Subscriptions Transport**:
   * Add WebSocket transport (`transport.Websockets`) to `server.go` using Redis Pub/Sub backend so mobile and web clients can subscribe to live alerts.

---

### 2.2 Event Publishing & The Transactional Outbox Pattern

Currently, when `post-service.CreatePost` or `follow-service.FollowUser` executes, it only writes to PostgreSQL. `feed-service` and `notification-service` are starved of events.

#### The Dual-Write Failure Mode:
If `post-service` writes to PostgreSQL and then immediately calls `redis.XAdd(...)`:
* If Redis is momentarily unreachable, the DB transaction commits, but the event is permanently lost. Followers will never see the post in their feeds.
* If Redis succeeds first, but the DB rollback occurs, phantom notifications are sent for a post that does not exist.

#### The Solution: Transactional Outbox Pattern
```
┌──────────────────────────────────────────────────────────────────┐
│                         POST SERVICE                             │
│                                                                  │
│  BEGIN TRANSACTION;                                              │
│    1. INSERT INTO posts (...) VALUES (...);                      │
│    2. INSERT INTO outbox_events (event_type, payload, status)   │
│       VALUES ('post.created', '{"post_id": "...", ...}', 'NEW'); │
│  COMMIT;                                                         │
└─────────────────────────────────┬────────────────────────────────┘
                                  │ (Guaranteed Atomic in Postgres)
                                  ▼
┌──────────────────────────────────────────────────────────────────┐
│                   OUTBOX RELAY WORKER (CDC / Poller)             │
│                                                                  │
│  - Reads 'NEW' events with SELECT FOR UPDATE SKIP LOCKED         │
│  - Publishes to Redis Stream (social:events)                     │
│  - Marks status = 'PUBLISHED' (or Debezium reads WAL logs)       │
└─────────────────────────────────┬────────────────────────────────┘
                                  │
                                  ▼
┌──────────────────────────────────────────────────────────────────┐
│                    REDIS STREAM: social:events                   │
└──────────────────────────────────────────────────────────────────┘
```

1. Create `outbox_events` table in PostgreSQL for `post-service` and `follow-service`:
   ```sql
   CREATE TABLE outbox_events (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       event_type VARCHAR(64) NOT NULL,
       aggregate_id UUID NOT NULL,
       payload JSONB NOT NULL,
       status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
       created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
   );
   CREATE INDEX idx_outbox_pending ON outbox_events (created_at) WHERE status = 'PENDING';
   ```
2. Implement an asynchronous poller or use Debezium (Postgres CDC) to stream committed events into Redis Streams with zero data loss.

---

### 2.3 Asynchronous Media Upload Pipeline

Currently, `post-service` accepts a base64-encoded file string and uploads synchronously to Cloudinary within `CreatePost`:
* **Bottleneck**: Base64 strings increase payload size by 33%.
* **Thread Block**: A 5MB image upload holds a Go gRPC worker thread open for 1–3 seconds.
* **Failure Risk**: Any Cloudinary latency spike directly stalls API Gateway responses.

#### Target Architecture: Direct Client Upload via Pre-Signed URLs
```
[Client App]                     [API Gateway]                [Media Worker / S3]
     │                                │                                │
     ├── 1. RequestUploadUrl(type) ──▶│                                │
     │   (image/jpeg, 4.2MB)          ├── 2. Generate Pre-signed URL ──▶
     │                                │◀─ 3. Return S3/Cloudinary URL ─┤
     │◀─ 4. Pre-Signed PUT URL ───────┤                                │
     │                                                                 │
     ├── 5. Direct Upload PUT ────────────────────────────────────────▶│ (S3 Bucket)
     │   (Bypasses our backend servers completely)                     │
     │◀─ 6. 200 OK ────────────────────────────────────────────────────┤
     │                                                                 │
     ├── 7. CreatePost(media_key) ───▶ [Post Service]                  │
     │                                 (Saves lightweight reference)   │
```

---

### 2.4 Missing Core Services & Features

To complete a modern social platform, the following features and services must be added:

1. **Comment Service (`comment-service` or sub-module in `post-service`)**:
   * Hierarchical/nested comments (threaded replies).
   * Comment likes, mentions (`@username`), and comment count denormalization on posts.
   * Emits `CommentCreatedEvent` and `CommentLikedEvent`.
2. **Search & Discovery Service (`search-service`)**:
   * Full-text search for posts by keywords and hashtags (`#golang`, `#systemdesign`).
   * User auto-complete search by username or display name.
   * Backed by PostgreSQL `pg_trgm` / `tsvector` or Elasticsearch/Meilisearch.
3. **Direct Messaging (Chat) Service (`chat-service`)**:
   * 1-on-1 and group messaging.
   * WebSocket state management for presence (online/offline status) and typing indicators.
   * Durable storage in ScyllaDB or MongoDB optimized for sequential chat logs.
4. **User Moderation & Safety**:
   * Block and mute relationship graph.
   * Flagging/reporting content for automated or manual review.

---

### 2.5 Authentication, Token Lifecycle & Session Revocation

#### What needs to be built:
1. **API Gateway Auth Middleware**:
   * Inspects `Authorization: Bearer <token>` on incoming GraphQL requests.
   * Parses and validates JWT using ECDSA (P-256) or Ed25519 public keys.
   * Injects `user_id`, `role`, and `scopes` into `context.Context`.
   * Passes claims downstream to microservices via gRPC Metadata:
     ```go
     md := metadata.Pairs("x-user-id", claims.UserID, "x-user-role", claims.Role)
     ctx = metadata.NewOutgoingContext(ctx, md)
     ```
2. **Session Revocation via Redis Blacklist**:
   * Keep JWT access token lifetime short (15 minutes).
   * For instant user logout or password reset, store revoked token IDs (`jti`) in Redis with TTL matching token expiration:
     `SET token:blacklist:<jti> 1 EX 900`
   * Gateway checks this key in Redis cache before allowing authenticated requests.

---

## 3. Architecting for Millions of Users (High Concurrency & Scale-Out)

Scaling to millions of active users requires treating every microservice as horizontally stateless, removing single points of failure, decoupling read paths from relational databases, and isolating noisy neighbors.

### 3.1 Target Scale & Traffic Math (50,000+ QPS)

Let us dimension the infrastructure for **10 Million Daily Active Users (DAU)**:

* **Daily Actions**:
  * 10M users generate an average of 40 read requests/day $\rightarrow$ **400 Million reads/day**.
  * 10M users generate an average of 4 write interactions/day $\rightarrow$ **40 Million writes/day**.
* **Traffic Demands**:
  * **Average Read QPS**:
    $$\text{Avg Read QPS} = \frac{400,000,000}{86,400} \approx 4,630 \text{ req/sec}$$
  * **Peak Read QPS (5x peak factor)**: $\approx \mathbf{23,000 \text{ req/sec}}$
  * **Peak Write QPS (5x peak factor)**: $\approx \mathbf{2,300 \text{ req/sec}}$
  * **WebSocket Connections**: Concurrent active connections $\approx 10\% \text{ of DAU} = \mathbf{1,000,000 \text{ simultaneous sockets}}$.
* **Data Growth**:
  * Posts created: 5 Million posts/day $\times 1\text{ KB} \approx 5\text{ GB/day} \rightarrow 1.8\text{ TB/year}$.
  * Notifications created: 30 Million/day $\times 500\text{ bytes} \approx 15\text{ GB/day}$.
  * Feeds: 10M active timelines $\times 800\text{ post references} \times 40\text{ bytes} \approx \mathbf{320\text{ GB RAM in Redis}}$.

---

### 3.2 Edge, CDN & Layer-7 Load Balancing

```
                             [Global Users]
                                   │
                                   ▼
                    ┌──────────────────────────────┐
                    │ Anycast DNS (Cloudflare CDN) │
                    │ - Static Media Caching (99%) │
                    │ - WAF & DDoS Shielding       │
                    │ - TLS Termination (HTTP/3)   │
                    └──────────────┬───────────────┘
                                   │
                                   ▼
                    ┌──────────────────────────────┐
                    │ Cloud Load Balancer (Envoy)  │
                    │ - Rate Limiting per IP/User  │
                    │ - Path-Based Routing         │
                    └──────┬────────────────┬──────┘
                           │                │
            /graphql, /api │                │ /ws (WebSockets)
                           ▼                ▼
                  ┌────────────────┐  ┌────────────────┐
                  │  API Gateway   │  │   WS Gateway   │
                  │   Cluster      │  │   Cluster      │
                  │  (20-50 Pods)  │  │  (30-80 Pods)  │
                  └────────────────┘  └────────────────┘
```

1. **CDN Edge Caching**:
   * Route Cloudinary/S3 assets through Cloudflare CDN. Cache avatars, post images, and static videos with a 30-day cache header (`Cache-Control: public, max-age=2592000, immutable`).
   * This offloads 95% of media bandwidth from backend infrastructure.
2. **Layer-7 Load Balancing (Envoy / NGINX Ingress)**:
   * Terminate TLS at the edge.
   * Route HTTP/GraphQL traffic to `api-gateway` pods.
   * Route persistent WebSocket connections (`/ws/notifications`) to `notification-service` WebSocket gateway pods with extended read/write timeouts (1 hour).

---

### 3.3 Database Scaling: Read/Write Splitting, Partitioning & Citus Sharding

Single-node PostgreSQL databases crash when concurrent query volume exceeds 5,000 QPS. To scale PostgreSQL to millions of users:

#### 1. Read/Write Connection Splitting
* **Primary Instance**: Handles all `INSERT`, `UPDATE`, `DELETE` operations.
* **Read Replicas (3 to 5 nodes)**: Handle all read queries (`SELECT ... WHERE author_id = ...`).
* **Connection Pooling**: Use **PgBouncer** in transaction pooling mode in front of each PostgreSQL instance. This allows 50,000 backend goroutines to share 200 physical database connections without OS context-switching overhead.

```
                  ┌────────────────────────────────────────┐
                  │             Microservice               │
                  └───┬────────────────────────────────┬───┘
                      │ Writes                         │ Reads
                      ▼                                ▼
              ┌───────────────┐                ┌───────────────┐
              │ PgBouncer (W) │                │ PgBouncer (R) │
              └───────┬───────┘                └───────┬───────┘
                      ▼                                ▼
              ┌───────────────┐ Streaming      ┌───────────────┐
              │  PostgreSQL   │ Replication    │  PostgreSQL   │
              │   (Primary)   ├───────────────▶│ (Read Replica)│
              └───────────────┘                └───────────────┘
```

#### 2. Table Partitioning by Time & ID
Partition the most read/write-heavy tables:
* `notifications`: Partition by Range on `created_at` (monthly partitions). Allows dropping data older than 90 days with instant `DROP TABLE` instead of heavy `DELETE` scans.
* `posts`: Partition by Hash on `author_id` or Range on `created_at`.

#### 3. Citus / Distributed Sharding (When Dataset > 5 TB)
When relational data exceeds 5TB, shard user data across PostgreSQL cluster worker nodes using **Citus Data**:
* Shard key: `user_id` or `author_id`.
* All user tables (`users`, `posts`, `follows`) co-locate their shards by `user_id`, ensuring joins and transactions remain local to single nodes.

---

### 3.4 Caching Architecture: Singleflight, Thundering Herds & Redis Cluster

Under heavy read spikes (e.g. an influential user posts an announcement), thousands of simultaneous requests for the same user profile or post will bypass an expired cache and overwhelm the database.

#### 1. Singleflight (Request Deduplication)
Use Go's `golang.org/x/sync/singleflight` package in services. If 10,000 concurrent requests request `GetUserById("user-123")` and cache misses, **only one** database query is executed. The remaining 9,999 callers await that single result:

```go
var userGroup singleflight.Group

func (s *userService) GetUserById(ctx context.Context, id string) (*models.User, error) {
    // 1. Try Redis Cache
    if user, err := s.cache.Get(ctx, id); err == nil {
        return user, nil
    }

    // 2. Deduplicate simultaneous DB hits
    v, err, _ := userGroup.Do(id, func() (interface{}, error) {
        user, err := s.repo.GetUserById(ctx, id)
        if err != nil {
            return nil, err
        }
        _ = s.cache.Set(ctx, id, user, 10*time.Minute)
        return user, nil
    })

    if err != nil {
        return nil, err
    }
    return v.(*models.User), nil
}
```

#### 2. Probabilistic Early Expiration (XFetch)
Instead of hard TTLs where the cache suddenly goes cold, workers compute an early expiration score:
$$\Delta \times \beta \times \ln(\text{rand}())$$
If the random threshold triggers within the last 10% of TTL, the current worker asynchronously refreshes the cache in the background while instantly returning the existing value to the user.

#### 3. Redis Cluster Mode
Deploy Redis in **Cluster Mode** across 3 availability zones with 6 primary shards and 6 read replicas:
* Sharding key tags ensure related keys hash to the same slot: `{user:123}:timeline` and `{user:123}:unread`.

---

### 3.5 Event Bus Evolution: Redis Streams to Apache Kafka / Redpanda

Redis Streams is ideal for early-stage and medium-scale systems ($< 50,000 \text{ events/sec}$) because it requires no external infrastructure beyond Redis. However, as SocialFlow scales past 10M DAU:

| Attribute | Redis Streams (Current) | Apache Kafka / Redpanda (Target at Scale) |
| :--- | :--- | :--- |
| **Storage Medium** | In-Memory RAM | Disk / NVMe with OS page caching |
| **Event Retention** | Days (limited by expensive RAM) | Weeks or Months (inexpensive disk) |
| **Partition Scaling** | 1 Stream per topic; sharded manually | Native horizontal partitions (e.g. 64 partitions/topic) |
| **Consumer Throughput** | ~50k msgs/sec | > 1M msgs/sec |
| **Replayability** | Good for short windows | Long-term replay for analytics and new service backfills |

**Migration Trigger**: Transition from Redis Streams to Kafka/Redpanda when Redis RAM utilization for streams exceeds 64GB or daily event volume exceeds 100 Million events.

---

### 3.6 Millions of WebSocket Connections (Gateway Pod Sharding)

Maintaining 1,000,000 persistent WebSocket connections requires solving OS socket limits and distributed message delivery:

1. **Linux Kernel Socket Optimization (`/etc/sysctl.conf`)**:
   ```ini
   fs.file-max = 2097152
   net.ipv4.ip_local_port_range = 1024 65535
   net.core.somaxconn = 65535
   net.ipv4.tcp_max_syn_backlog = 65535
   net.ipv4.tcp_tw_reuse = 1
   ```
2. **Pod Sizing**:
   * Each idle WebSocket connection in Go consumes ~4–8 KB of RAM (goroutine stacks + read/write buffers).
   * 1,000,000 connections $\approx 6\text{ to }8\text{ GB RAM}$.
   * Deploy **40 WebSocket Gateway Pods**, each managing ~25,000 connections.
3. **Redis Pub/Sub Connection Fanout**:
   * Pods subscribe dynamically to `notify:<user_id>` when a client connects and unsubscribe upon disconnect.
   * Real-time dispatch latency remains $< 5\text{ms}$ regardless of which pod holds the socket.

---

## 4. Performance Optimizations (DB, Memory, Go Runtime)

### 4.1 PostgreSQL Engine Tuning & Indexing Strategies

#### Essential Indexes Checklist:

```sql
-- 1. Post Queries: Fast user timeline and public feeds
CREATE INDEX idx_posts_author_created 
    ON posts (author_id, created_at DESC) 
    WHERE is_public = TRUE;

-- 2. Follow Queries: Instant bi-directional graph lookup
CREATE UNIQUE INDEX idx_follow_pair 
    ON follows (follower_id, following_id);
CREATE INDEX idx_follow_following 
    ON follows (following_id);

-- 3. Notification Queries: Composite cursor pagination & unread filter
CREATE INDEX idx_notifications_inbox 
    ON notifications (recipient_id, created_at DESC);
CREATE INDEX idx_notifications_unread_fast 
    ON notifications (recipient_id) 
    WHERE is_read = FALSE;

-- 4. User Queries: Fast login lookups
CREATE UNIQUE INDEX idx_users_email_lower ON users (LOWER(email));
CREATE UNIQUE INDEX idx_users_username_lower ON users (LOWER(username));
```

#### PostgreSQL Parameter Tuning (`postgresql.conf` for 64GB RAM instance):
* `max_connections = 300` (Use PgBouncer for thousands of app connections).
* `shared_buffers = 16GB` (25% of total system RAM).
* `effective_cache_size = 48GB` (75% of total system RAM).
* `work_mem = 64MB` (Prevents queries from spilling sorts to disk).
* `maintenance_work_mem = 2GB`.
* `random_page_cost = 1.1` (Tuned for modern NVMe SSDs; avoids excessive index scan penalties).

---

### 4.2 Go Runtime, Garbage Collector & Memory Tuning

Under heavy concurrency, Go's Garbage Collector (GC) can trigger stop-the-world pauses if heap allocations churn rapidly.

1. **`GOMEMLIMIT` & `GOGC` Tuning**:
   * Set `GOMEMLIMIT` to 90% of the container's memory limit (e.g. `GOMEMLIMIT=3600MiB` on a 4GB pod).
   * This prevents out-of-memory (OOM) kills by running GC aggressively only when approaching the limit, rather than at rigid percentage increments.
2. **Buffer Pooling with `sync.Pool`**:
   * In WebSocket handlers and JSON/Protobuf serialization, reuse buffers:
     ```go
     var bufferPool = sync.Pool{
         New: func() interface{} {
             return new(bytes.Buffer)
         },
     }
     ```
3. **Pre-allocating Slices**:
   * Avoid repeated slice resizing allocations in feed hydration and batch queries:
     ```go
     // Bad: Triggers 8 slice reallocation copies
     var postIDs []string
     for _, p := range posts { postIDs = append(postIDs, p.ID) }

     // Good: Exactly 1 heap allocation
     postIDs := make([]string, 0, len(posts))
     for _, p := range posts { postIDs = append(postIDs, p.ID) }
     ```

---

### 4.3 gRPC & Protocol Buffer Throughput Tuning

1. **HTTP/2 Subchannel Connection Pooling**:
   * By default, a gRPC client uses a single HTTP/2 TCP connection. A single TCP stream can become CPU or TCP window bottlenecked at $> 15,000$ QPS.
   * Implement a client connection pool in `api-gateway` maintaining 4–8 gRPC sub-connections per microservice.
2. **gRPC Keepalive Configuration**:
   ```go
   kpc := keepalive.ClientParameters{
       Time:                10 * time.Second,
       Timeout:             3 * time.Second,
       PermitWithoutStream: true,
   }
   grpc.Dial(target, grpc.WithKeepaliveParams(kpc))
   ```

---

### 4.4 GraphQL Dataloader (Eliminating N+1 Queries)

In `api-gateway`, resolving author profiles for 20 posts in a feed query naively causes 20 separate gRPC calls to `user-service`.

#### Solution: Dataloader Batching
Use `graph/dataloader` (or `gqlgen` integration):
* Within a single GraphQL query resolution tick (e.g., 2ms window), the dataloader collects all 20 `author_id`s.
* Executes a single batch gRPC call: `UserService.BatchGetUsers([id1, id2, ... id20])`.
* Distributes results back to individual field resolvers in memory.
* Reduces network serialization round-trips from 20 to 1.

---

## 5. Security, Resilience & Fault Tolerance

### 5.1 Circuit Breaking, Timeouts & Deadlines

When downstream services (e.g. `post-service`) experience high latency, upstream callers must fail fast rather than holding connections open until exhaustion.

1. **Context Deadline Propagation**:
   * Every request entering `api-gateway` must carry a root timeout context:
     ```go
     ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
     defer cancel()
     ```
   * gRPC automatically propagates this deadline in HTTP/2 headers (`grpc-timeout`). If downstream takes $> 3\text{s}$, it cancels ongoing work immediately.
2. **Circuit Breaking (`sony/gobreaker`)**:
   * Wrap inter-service gRPC calls in a circuit breaker.
   * If failure rate exceeds 20% over 10 seconds, trip the breaker to **Open**.
   * Fall back immediately (e.g. return cached feeds or empty notifications) without hammering the failing service.

---

### 5.2 Distributed Rate Limiting & Bot Defense

Prevent scrapers, automated like bots, and DDoS flooding:

1. **Sliding Window Rate Limiting in Redis**:
   * **Authentication endpoints** (`/login`, `/register`): Max 5 requests / minute per IP.
   * **Post creation** (`/createPost`): Max 10 posts / minute per user.
   * **Feed retrieval** (`/getFeed`): Max 120 requests / minute per user.
2. **Token Bucket Algorithm**:
   * Implemented via Redis Lua scripts to execute atomic balance checks in a single round-trip.

---

### 5.3 Graceful Shutdown & Zero-Downtime Deployments

When Kubernetes rotates or scales down a pod:
1. Pod receives `SIGTERM`.
2. Stop accepting new HTTP/gRPC requests (fail readiness probe).
3. Allow active requests up to 15 seconds to complete.
4. Stop event consumer loops and flush pending offsets (`XAck`).
5. Close database connection pools cleanly.
6. Exit process with code 0.

---

## 6. Observability, Tracing & Metrics

A microservices system with 6+ services cannot be debugged through local console logs.

### 6.1 OpenTelemetry Distributed Tracing (Correlation IDs)

```
[Client] ──Request──▶ [API Gateway] (Generates Trace ID: "c18f...")
                            │
               gRPC Metadata: "x-trace-id: c18f..."
                            │
                            ├───▶ [Feed Service] (Span: FetchTimeline)
                            │
                            └───▶ [Post Service] (Span: BatchGetPosts)
```

1. **Trace Header Propagation**:
   * API Gateway extracts or generates a `traceparent` (W3C standard) or `x-trace-id`.
   * Injects the trace context into gRPC metadata using OpenTelemetry gRPC interceptors.
2. **Jaeger / Grafana Tempo Integration**:
   * Visualizes the complete waterfall of a request across all services to instantly spot the bottleneck function.

---

### 6.2 Prometheus Metrics Blueprint

Every microservice exposes `/metrics` on an internal admin port (e.g. `:9090`):

| Metric Name | Type | Labels | Description |
| :--- | :--- | :--- | :--- |
| `http_requests_total` | Counter | `service`, `handler`, `status` | Total incoming HTTP/GraphQL requests. |
| `grpc_server_handling_seconds` | Histogram | `service`, `grpc_method`, `code` | Latency distribution of gRPC endpoints (p50, p95, p99). |
| `db_pool_connections_active` | Gauge | `service`, `pool` | Number of physical DB connections currently in use. |
| `redis_stream_consumer_lag` | Gauge | `stream`, `group` | Unprocessed events backlog (critical alert if $> 1000$). |
| `websocket_active_connections` | Gauge | `pod_id` | Number of concurrent live WebSockets per pod. |
| `notification_dispatch_total` | Counter | `channel`, `status` | Count of real-time, push, and email dispatches. |

---

### 6.3 Structured Logging with `log/slog`

All logs must be structured JSON output to `stdout` for ingestion by FluentBit / Vector into Loki or Elasticsearch:

```json
{
  "time": "2026-11-03T18:45:00.123Z",
  "level": "ERROR",
  "msg": "failed to hydrate feed posts",
  "service": "feed-service",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "user_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "error": "post-service unavailable: connection refused",
  "latency_ms": 142
}
```

---

## 7. Comprehensive Testing Blueprint & Exact Test Cases for Every Service

A resilient social network requires testing across every tier of the testing pyramid:
1. **Unit Tests**: Mocked dependencies, zero network I/O, lightning-fast execution.
2. **Integration Tests**: Real PostgreSQL and Redis instances running via `testcontainers-go`.
3. **End-to-End (E2E) Tests**: Full lifecycle validation simulating multi-user interaction.
4. **Load & Stress Tests**: Concurrency saturation using `k6`.

---

### 7.1 Testing Strategy Matrix

```
       ▲
      / \        End-to-End Tests (10%)
     /   \       Full platform flows, API Gateway to DB
    /     \
   /       \     Integration Tests (30%)
  /         \    PostgreSQL queries, Redis Streams, gRPC servers
 /           \
/─────────────\  Unit Tests (60%)
                 Business logic, validation, aggregation, policies
```

---

### 7.2 Service 1: `user-service` Test Cases

#### Unit Tests (`internal/service/user_service_test.go`):
* [ ] **`TestRegister_Success`**: Valid input creates user, hashes password with bcrypt (cost 12), and returns user without exposing password hash.
* [ ] **`TestRegister_DuplicateEmail`**: Returns `ErrEmailAlreadyExists` when email exists.
* [ ] **`TestRegister_DuplicateUsername`**: Returns `ErrUsernameAlreadyExists` when handle is taken.
* [ ] **`TestRegister_InvalidEmailFormat`**: Rejects invalid emails (e.g. `invalid-email`, `@domain.com`).
* [ ] **`TestRegister_WeakPassword`**: Rejects passwords $< 8$ characters or lacking complexity.
* [ ] **`TestLogin_Success`**: Valid email/password returns signed Access Token and Refresh Token with correct claims and expiration.
* [ ] **`TestLogin_InvalidPassword`**: Returns `ErrInvalidCredentials` on mismatched hash; does not leak whether email exists.
* [ ] **`TestLogin_NonExistentEmail`**: Returns `ErrInvalidCredentials` for unknown email.
* [ ] **`TestTokenRefresh_Success`**: Valid refresh token returns a new access token and rotated refresh token.
* [ ] **`TestTokenRefresh_RevokedToken`**: Rejects refresh token if blacklisted in Redis.
* [ ] **`TestUpdateUser_PartialUpdate`**: Updates only display name without overwriting email or password.
* [ ] **`TestUpdateUser_Unauthorized`**: Prevents User A from updating User B's profile.

#### Integration Tests (`internal/repository/user_repository_test.go`):
* [ ] **`TestPostgres_UserConstraints`**: Verifies database-level uniqueness on `users.email` and `users.username`.
* [ ] **`TestPostgres_SoftDelete`**: Verifies deleted user is omitted from standard lookups but preserved for audit.

---

### 7.3 Service 2: `post-service` Test Cases

#### Unit Tests (`internal/services/post_service_test.go`):
* [ ] **`TestCreatePost_Success`**: Valid post content persists post and generates unique UUID and timestamp.
* [ ] **`TestCreatePost_EmptyContent`**: Rejects post with empty content and no media.
* [ ] **`TestCreatePost_ExceedsCharacterLimit`**: Rejects text $> 1,000$ characters.
* [ ] **`TestUpdatePost_Success`**: Author successfully edits content and updates `updated_at`.
* [ ] **`TestUpdatePost_Forbidden`**: Returns `ErrUnauthorized` when non-author attempts update.
* [ ] **`TestDeletePost_Success`**: Author can delete post.
* [ ] **`TestDeletePost_Forbidden`**: Non-author cannot delete post.
* [ ] **`TestLikePost_Toggle`**: First call adds like (count + 1); second call removes like (count - 1).
* [ ] **`TestArchivePost_Success`**: Moves post to archived state; excluded from `GetAllPost` results.

#### Integration Tests & Event Publishing:
* [ ] **`TestPostgres_ConcurrentLikes`**: 50 simultaneous goroutines liking the same post correctly increments `like_count` to exactly 50 without lost updates.
* [ ] **`TestOutbox_PostCreatedEvent`**: Verifies creating a post atomically writes a `PostCreatedEvent` row to `outbox_events`.

---

### 7.4 Service 3: `follow-service` Test Cases

#### Unit Tests (`internal/services/follow_service_test.go`):
* [ ] **`TestFollowUser_Success`**: User A follows User B; creates relationship record.
* [ ] **`TestFollowUser_CannotFollowSelf`**: Returns `ErrCannotFollowSelf` when `follower_id == following_id`.
* [ ] **`TestFollowUser_Idempotent`**: Following a user already followed returns success without creating duplicate rows or erroring.
* [ ] **`TestUnfollowUser_Success`**: Removes relationship record; decrements counts.
* [ ] **`TestUnfollowUser_NotFollowing`**: Unfollowing someone not followed returns success cleanly.
* [ ] **`TestGetFollowers_Pagination`**: Returns correct slice of followers with cursor pagination.
* [ ] **`TestGetFollowCounts`**: Accurately aggregates follower and following counts.
* [ ] **`TestRemoveFollower`**: Allows creator to force a user to unfollow them.

#### Integration Tests:
* [ ] **`TestPostgres_BiDirectionalGraph`**: Tests mutual follow detection (`is_mutual = true` if A follows B and B follows A).
* [ ] **`TestOutbox_FollowEvent`**: Verifies `UserFollowedEvent` written to outbox table inside the same transaction.

---

### 7.5 Service 4: `feed-service` Test Cases

#### Unit Tests:
* [ ] **`TestFeedFanout_StandardAuthor`**: Author with 500 followers pushes `post_id` into all 500 follower Redis `ZSET`s.
* [ ] **`TestFeedFanout_CelebrityBypass`**: Author with $> 25,000$ followers does **not** push to follower timelines; post is stored only in celebrity's public post list.
* [ ] **`TestFeedFanout_InactiveFilter`**: Followers inactive for $> 14$ days are skipped during fanout to save memory.
* [ ] **`TestTimeline_MaxCapacityTrim`**: Timelines are capped at 800 items via `ZREMRANGEBYRANK timeline:<user_id> 0 -801`.
* [ ] **`TestGetFeed_HydrationMerge`**: Combines pushed timeline items with pulled celebrity posts, sorts descending by timestamp, and hydrates full post and author data.
* [ ] **`TestGetFeed_GhostPostFilter`**: Posts deleted in `post-service` are filtered out during hydration and lazily purged from Redis.

#### Integration Tests (`internal/events/consumer_test.go`):
* [ ] **`TestRedisConsumer_AutoClaim`**: Simulates a crashed worker; verifies a secondary worker reclaims pending unacknowledged messages via `XAutoClaim`.
* [ ] **`TestRedisConsumer_PoisonMessage`**: Malformed event payload is logged, acknowledged (`XAck`), and does not block the consumer loop.

---

### 7.6 Service 5: `notification-service` Test Cases

#### Unit Tests:
* [ ] **`TestSelfActionFilter`**: User liking their own post or commenting on their own thread generates zero notifications.
* [ ] **`TestDeduplication`**: Duplicate `PostLikedEvent` within 60 seconds is dropped via `SET NX` idempotency key.
* [ ] **`TestAggregation_LikeSurge`**:
  * Event 1 from Alice: Notification created with title *"Alice liked your post"*.
  * Event 2 from Bob: Aggregated to *"Alice and Bob liked your post"*.
  * Event 3 from Carol: Aggregated to *"Alice, Bob, and 1 other liked your post"*.
  * Event 20 from Dave: Aggregated to *"Alice, Bob, and 18 others liked your post"*.
* [ ] **`TestPolicy_QuietHours`**: Push notification suppressed when current recipient time falls between 22:00 and 07:00 local time.
* [ ] **`TestPolicy_MutedCategory`**: In-app and push skipped when user disabled likes in settings.

#### Integration Tests:
* [ ] **`TestWebSocket_MultiPodDelivery`**: User connects to WS Pod 1; worker publishes to Redis Pub/Sub; WS Pod 1 receives and pushes JSON frame over client socket.
* [ ] **`TestBadgeCounter_AtomicSync`**: Verify `INCR unread:<user_id>` on insert and `DECRBY` on batch mark-as-read.
* [ ] **`TestPushToken_Invalidation`**: When APNs returns `410 Gone`, token is marked `is_active = FALSE` in DB.

---

### 7.7 Service 6: `api-gateway` Test Cases

#### Unit Tests:
* [ ] **`TestAuthDirective_MissingToken`**: Rejects protected query with `UNAUTHENTICATED` error code.
* [ ] **`TestAuthDirective_ExpiredToken`**: Rejects token past expiration.
* [ ] **`TestQueryComplexity_Limit`**: Rejects deeply nested cyclic GraphQL queries exceeding max depth of 5.

#### Integration Tests:
* [ ] **`TestGateway_FeedQueryResolution`**: Full GraphQL query `query { getFeed { posts { id content author { name } } } }` resolves correctly across `feed-service`, `post-service`, and `user-service`.
* [ ] **`TestGateway_DataloaderBatching`**: Verifies that 20 post author fields trigger exactly 1 batched gRPC call to `user-service`.

---

### 7.8 End-to-End & Concurrency Stress Test Cases

```
┌────────────────────────────────────────────────────────────────────────┐
│                        END-TO-END VERIFICATION FLOW                   │
│                                                                        │
│  1. Register User A & User B via GraphQL                               │
│  2. User B follows User A                                              │
│  3. User B establishes real-time WebSocket connection to Gateway       │
│  4. User A publishes a new post                                        │
│  5. Assert: PostCreated event emitted to Redis Stream                  │
│  6. Assert: Post appears in User B's feed within 1.5 seconds           │
│  7. User B likes User A's post                                         │
│  8. Assert: User A receives real-time WebSocket alert:                 │
│     "User B liked your post" with unread_count = 1                     │
│  9. User A calls MarkAsRead                                            │
│ 10. Assert: unread_count resets to 0 across Redis and PostgreSQL       │
└────────────────────────────────────────────────────────────────────────┘
```

---

### 7.9 k6 Load Testing Script Templates

To stress test the system under high concurrency, use `k6` scripts simulating realistic user behavior.

#### Feed Retrieval Benchmark (`tests/load/feed_load.js`):
```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '1m', target: 500 },   // Ramp-up to 500 users
        { duration: '3m', target: 2000 },  // Scale to 2,000 concurrent users
        { duration: '1m', target: 0 },     // Ramp-down
    ],
    thresholds: {
        http_req_duration: ['p(95)<100', 'p(99)<250'], // 95% under 100ms
        http_req_failed: ['rate<0.01'],                 // Error rate < 1%
    },
};

const GRAPHQL_ENDPOINT = 'http://localhost:8080/query';

export default function () {
    const query = `
        query GetFeed($limit: Int, $cursor: String) {
            getFeed(limit: $limit, cursor: $cursor) {
                posts {
                    id
                    content
                    mediaUrl
                    likeCount
                    author {
                        username
                        name
                    }
                }
                nextCursor
                hasMore
            }
        }
    `;

    const payload = JSON.stringify({
        query: query,
        variables: { limit: 20 },
    });

    const params = {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer <TEST_JWT_TOKEN>',
        },
    };

    const res = http.post(GRAPHQL_ENDPOINT, payload, params);
    check(res, {
        'status is 200': (r) => r.status === 200,
        'has posts': (r) => JSON.parse(r.body).data.getFeed.posts.length > 0,
    });

    sleep(1);
}
```

---

## 8. Phased Implementation Roadmap & Prioritized Execution Checklist

| Phase | Priority | Milestone / Task | Estimated Effort | Impact |
| :--- | :--- | :--- | :--- | :--- |
| **Phase 1** | **P0 (Immediate)** | **Wire Event Publishers**: Add outbox event emission to `post-service` and `follow-service` so domain events reach Redis Streams. | 2 Days | **Critical**: Unblocks feed and notification processing. |
| **Phase 1** | **P0 (Immediate)** | **Wire API Gateway Resolvers**: Connect `feed-service` and `notification-service` gRPC clients into `api-gateway` and define GraphQL types. | 2 Days | **Critical**: Makes feeds and notifications accessible to users. |
| **Phase 1** | **P0 (Immediate)** | **JWT Auth Middleware**: Implement token validation in `api-gateway` and propagate `x-user-id` downstream via gRPC metadata. | 1 Day | **Critical**: Secures endpoints against unauthorized tampering. |
| **Phase 2** | **P1 (High)** | **Core Unit & Integration Test Suites**: Implement unit tests for `user-service`, `post-service`, and `follow-service` using the test specifications above. | 3 Days | **High**: Prevents regressions and guarantees logic correctness. |
| **Phase 2** | **P1 (High)** | **GraphQL Dataloader**: Add batching dataloaders in `api-gateway` for author profile and like states to eliminate N+1 queries. | 2 Days | **High**: Reduces read latency by 70%. |
| **Phase 2** | **P1 (High)** | **Pre-Signed Upload URL Pipeline**: Refactor media uploads in `post-service` to direct client S3/Cloudinary pre-signed URLs. | 2 Days | **High**: Eliminates memory crashes and request stalls. |
| **Phase 3** | **P2 (Scale)** | **Database Scaling & Connection Pooling**: Deploy PgBouncer in front of PostgreSQL; configure read/write connection splitting in repositories. | 3 Days | **Scale**: Enables scaling past 5,000 QPS without DB connection exhaustion. |
| **Phase 3** | **P2 (Scale)** | **Singleflight & Caching Layer**: Implement `singleflight.Group` and Redis caching for user profiles and post lookups. | 2 Days | **Scale**: Prevents thundering herds during traffic spikes. |
| **Phase 4** | **P3 (Production)** | **Observability Suite**: Instrument OpenTelemetry distributed tracing and Prometheus metrics across all gRPC handlers and GraphQL resolvers. | 3 Days | **Production**: Provides full system transparency and latency profiling. |
| **Phase 4** | **P3 (Production)** | **Kubernetes Manifests & HPA**: Containerize all services with multi-stage Dockerfiles and deploy HPA configurations on EKS/GKE. | 4 Days | **Production**: Enables automated horizontal auto-scaling from 10 to 100 pods. |
| **Phase 5** | **P4 (Next Gen)** | **Kafka Migration & Citus Sharding**: Transition event broker to Redpanda/Kafka and shard PostgreSQL tables when active user base exceeds 10 Million. | 2 Weeks | **Long-Term**: Supports 100M+ global users. |

