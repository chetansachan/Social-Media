# Notification Service

The **Notification Service** is a high-throughput, low-latency microservice in the **SocialFlow** platform responsible for multi-channel user alerts (In-App Real-time WebSockets, Push Notifications via FCM/APNs, and transactional emails). It asynchronously captures platform events, aggregates repeated interactions, enforces user notification preferences, and synchronizes unread badge counters across user devices.

> [!NOTE]
> For the complete, in-depth architectural blueprint, capacity estimations, failure-mode analyses, and distributed systems design, refer to [SYSTEM_DESIGN.md](file:///c:/Users/Chetan/Documents/Developer/Projects/Social-Media/notification-service/SYSTEM_DESIGN.md).

---

## 1. Key Capabilities

* **Real-Time In-App Delivery**: Sub-200ms notification delivery over persistent WebSockets using a distributed Redis Pub/Sub cluster backplane.
* **Smart Aggregation & Roll-Up**: Automatically groups repetitive actions within time windows (e.g. "Alice, Bob, and 12 others liked your post") to prevent notification storms.
* **Unread Badge Synchronization**: Atomic Redis counters (`INCR`, `DECRBY`) ensuring that badge icons stay accurate across mobile and web clients.
* **Self-Action & Block List Filtering**: Automatically drops self-interactions (liking one's own post) and enforces user block lists.
* **Multi-Channel Dispatching**: Modular dispatch pipeline routing to In-App, Mobile Push (FCM / APNs), and Email (SendGrid / SES).
* **User Preferences & Quiet Hours**: Granular opt-in/opt-out settings per category/channel and timezone-aware quiet hours.
* **At-Least-Once Delivery & Deduplication**: Redis Stream consumer groups with `XAutoClaim` recovery paired with 60-second atomic Redis idempotency locks.

---

## 2. Tech Stack

* **Language**: Go 1.26+
* **RPC Framework**: gRPC / Protocol Buffers (Internal communication with API Gateway)
* **Real-Time Transport**: WebSockets (`nhooyr.io/websocket` or `gorilla/websocket`)
* **Event Broker / Bus**: Redis Streams (`social:events` stream with consumer group `notification-service`)
* **Hot Cache & Distributed Pub/Sub**: Redis (Unread counters, Aggregation state, Connection routing)
* **Primary Database**: PostgreSQL 16+ (Partitioned tables for inbox persistence and history)

---

## 3. High-Level Architecture

```
                                  ┌────────────────────────┐
                                  │      Client Apps       │
                                  │  (Web / iOS / Android) │
                                  └────┬──────────────┬────┘
                                       │              │
                    GraphQL / gRPC Read│              │ WebSocket (Real-Time)
                                       ▼              ▼
                          ┌─────────────────┐   ┌──────────────────────────────┐
                          │   API Gateway   │   │  Notification WS Gateway     │
                          └────────┬────────┘   │  (Port 8085)                 │
                                   │            └──────────────┬───────────────┘
                                   │ gRPC                      │
                                   │ (Port 50055)              │ Redis Pub/Sub
┌──────────────────────────────────▼───────────────────────────┴─────────────────────────────────┐
│                                   NOTIFICATION SERVICE                                         │
│                                                                                                │
│  ┌───────────────────────┐   ┌────────────────────────┐   ┌─────────────────────────────────┐  │
│  │   Notification Query  │   │  Event Ingestion Engine│   │  Preference & Policy Engine     │  │
│  │      gRPC Server      │   │ (Redis Stream Consumer)│   │ (Muting, Channels, Quiet Hours) │  │
│  └───────────┬───────────┘   └───────────┬────────────┘   └────────────────┬────────────────┘  │
│              │                           │                                 │                   │
│              │                           ▼                                 │                   │
│              │               ┌────────────────────────┐                    │                   │
│              │               │  Smart Aggregator &    │◄───────────────────┘                   │
│              │               │  Deduplication Engine  │                                        │
│              │               └───────────┬────────────┘                                        │
│              │                           │                                                     │
│              ▼                           ▼                                                     │
│  ┌────────────────────────────────────────────────────┐   ┌─────────────────────────────────┐  │
│  │              Notification Data Engine              │   │   Channel Dispatcher Pipeline   │  │
│  │  ┌──────────────────────┐  ┌────────────────────┐  │   │  ┌───────────┐   ┌───────────┐  │  │
│  │  │  PostgreSQL Storage  │  │ Redis Badge &      │  │   │  │  Mobile   │   │   Email   │  │  │
│  │  │  (Inbox & History)   │  │ Cache Engine       │  │   │  │Push (FCM) │   │ Dispatcher│  │  │
│  │  └──────────────────────┘  └────────────────────┘  │   │  └─────┬─────┘   └─────┬─────┘  │  │
│  └────────────────────────────────────────────────────┘   └────────┼───────────────┼────────┘  │
└────────────────────────────────────────────────────────────────────┼───────────────┼───────────┘
                                ▲                                    │               │
                                │ Redis Streams                      ▼               ▼
                 ┌──────────────┴──────────────┐               ┌───────────┐   ┌───────────┐
                 │        social:events        │               │ Apple APNs│   │ SendGrid/ │
                 │ (events:post, events:follow)│               │ Google FCM│   │ Amazon SES│
                 └─────────────────────────────┘               └───────────┘   └───────────┘
```

---

## 4. Directory Structure

```
notification-service/
├── cmd/
│   └── server/
│       └── main.go                 # Service entrypoint (gRPC, WS, & Event Consumer)
├── internal/
│   ├── config/
│   │   └── config.go               # Configuration parsing from environment
│   ├── database/
│   │   ├── postgres.go             # PostgreSQL connection pool (pgx/v5)
│   │   └── redis.go                # Redis client setup (Pub/Sub + Streams)
│   ├── events/
│   │   ├── consumer.go             # Redis Stream consumer group + XAutoClaim + DLQ
│   │   └── event_handler.go        # Event routing and request construction
│   ├── handler/
│   │   ├── grpc_handler.go         # gRPC server methods (GetNotifications, MarkAsRead)
│   │   ├── ws_hub.go               # Per-user connection hub and Redis subscriptions
│   │   └── ws_handler.go           # WebSocket upgrader, JWT auth, and heartbeat
│   ├── models/
│   │   ├── notification.go         # Domain notification entity, rendering & validation
│   │   ├── events.go               # Incoming domain event schemas
│   │   ├── settings.go             # User preference and quiet hours model
│   │   ├── device.go               # Device token validation
│   │   ├── cursor.go               # Opaque keyset pagination cursor
│   │   ├── dispatcher.go           # Realtime/push/email message shapes
│   │   └── errors.go               # Shared sentinel errors
│   ├── repository/
│   │   ├── notification_repo.go    # PostgreSQL queries (cursor pagination, partitions)
│   │   ├── cache_repo.go           # Redis counters, dedup, aggregation & limits
│   │   ├── settings_repo.go        # User settings repository + JSON cache
│   │   ├── device_repo.go          # Push device token repository
│   │   └── clients.go              # User Service actor hydration
│   ├── services/
│   │   ├── notification_service.go # Core notification lifecycle orchestration
│   │   ├── aggregator.go           # Smart roll-up engine
│   │   ├── policy_service.go       # Preference & quiet hours evaluation
│   │   ├── ports.go                # Dependency interfaces
│   │   └── options.go              # Behavioural defaults
│   └── dispatchers/
│       ├── realtime_dispatcher.go  # Redis Pub/Sub dispatcher to WebSocket gateway
│       ├── push_dispatcher.go      # Bounded push worker pool + retry
│       ├── push_providers.go       # Log, FCM v1, and APNs HTTP/2 providers
│       └── email_dispatcher.go     # Transactional email worker + SMTP
├── migrations/
│   ├── 000001_create_notifications.up.sql
│   ├── 000002_create_user_notification_settings.up.sql
│   └── 000003_create_user_device_tokens.up.sql
├── .air.toml                       # Live-reloading tool configuration
├── .env.example                    # Environment variable template
├── go.mod                          # Go module definition
├── makefile                        # Build, test, and migration recipes
├── CODE_EXPLANATION.md             # Full file-by-file code walkthrough
├── README.md                       # Service overview
└── SYSTEM_DESIGN.md                # Exhaustive architecture & engineering specification
```

---

## 5. Environment Variables (`.env.example`)

```ini
# Server Configuration
PORT=50055
WS_PORT=8085
ENV=development

# Database (PostgreSQL)
DATABASE_URL=postgres://postgres:postgres@localhost:5432/socialflow_notifications?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=10
DB_CONN_MAX_LIFETIME=5m

# Cache & Message Broker (Redis)
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0

# Event Ingestion (Redis Streams)
EVENT_CONSUMER_ENABLED=true
EVENT_STREAM=social:events
EVENT_GROUP=notification-service
EVENT_CONSUMER_NAME=notification-service-1
EVENT_CLAIM_INTERVAL_SEC=30

# Upstream Services (gRPC)
USER_SERVICE_ADDR=localhost:50051
POST_SERVICE_ADDR=localhost:50052
FOLLOW_SERVICE_ADDR=localhost:50053

# Aggregation & Rate Limiting
AGGREGATION_WINDOW_MINUTES=15
DEDUP_TTL_SECONDS=60
MAX_PUSH_PER_HOUR=5

# Push Providers (FCM / APNs)
FCM_CREDENTIALS_FILE=./config/fcm-service-account.json
APNS_KEY_FILE=./config/AuthKey_APNS.p8
APNS_KEY_ID=
APNS_TEAM_ID=
APNS_TOPIC=com.socialflow.app
```

---

## 6. gRPC API Interface

Defined in `proto/notification.proto`:

| RPC Method | Request | Response | Description |
| :--- | :--- | :--- | :--- |
| `GetNotifications` | `GetNotificationsRequest` | `GetNotificationsResponse` | Cursor-paginated inbox list with unread counter. |
| `GetUnreadCount` | `GetUnreadCountRequest` | `GetUnreadCountResponse` | Returns instant unread count cached in Redis. |
| `MarkAsRead` | `MarkAsReadRequest` | `MarkAsReadResponse` | Marks specified notification IDs as read and decrements counter. |
| `MarkAllAsRead` | `MarkAllAsReadRequest` | `MarkAllAsReadResponse` | Marks all notifications for a user as read and resets counter to 0. |
| `DeleteNotification`| `DeleteNotificationRequest` | `DeleteNotificationResponse` | Deletes a notification from user inbox. |
| `GetSettings` | `GetSettingsRequest` | `GetSettingsResponse` | Retrieves user channel & category preferences. |
| `UpdateSettings` | `UpdateSettingsRequest` | `UpdateSettingsResponse` | Updates preferences and quiet hours. |
| `RegisterDeviceToken`| `RegisterDeviceTokenRequest`| `RegisterDeviceTokenResponse`| Stores APNs/FCM token for mobile push. |
| `RevokeDeviceToken` | `RevokeDeviceTokenRequest` | `RevokeDeviceTokenResponse` | Invalidates token upon logout or app uninstall. |

---

## 7. Real-Time WebSocket Interface

* **Connection URL**: `ws://<HOST>:8085/ws/notifications?token=<JWT_TOKEN>`
* **Authentication**: JWT verified on WebSocket upgrade handshake.
* **Heartbeat**: 
  * Ping frame every 30s from server. Client must reply with pong frame.
* **Incoming Event Frame**:
```json
{
  "event": "notification_received",
  "data": {
    "id": "e3b0c442-98fc-1c14-9afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "type": "post_like",
    "title": "Alice and 3 others liked your post",
    "content_preview": "Excited to launch our new social architecture!",
    "action_url": "socialflow://posts/post-123",
    "unread_count": 4,
    "created_at": "2026-11-03T02:30:00Z"
  }
}
```

---

## 8. Next Steps for Implementation

1. **Add `notification.proto`**: Create protobuf contract in the `./proto` folder and generate Go stubs.
2. **Register in `go.work`**: Add `./notification-service` to workspace `go.work`.
3. **Initialize Service Module**: Run `go mod init github.com/chetansachan/socialflow/notification-service`.
4. **Create SQL Migrations**: Run migration scripts to create `notifications`, `user_notification_settings`, and `user_device_tokens`.
5. **Implement Event Consumers & WS Server**: Build the Redis Streams consumer and WebSocket gateway.

