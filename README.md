# SocialFlow — Distributed Microservices Social Media Platform

SocialFlow is a high-performance, event-driven social media backend built with **Go**, **gRPC**, **GraphQL**, **PostgreSQL**, and **Redis**. It is designed to scale horizontally to millions of active users with sub-millisecond in-memory caching, hybrid fanout feeds, and real-time WebSocket notifications.

---

## 📚 Core Architecture & Engineering Documentation

* 🚀 [**System Optimization, High-Scale Architecture & Testing Roadmap**](file:///c:/Users/Chetan/Documents/Developer/Projects/Social-Media/SYSTEM_OPTIMIZATION_SCALING_AND_TESTING_ROADMAP.md) — Comprehensive master guide covering what is left to build, scaling to millions of users, performance optimizations, database tuning, and the full test suite blueprint.
* 📰 [**Feed Service System Design**](file:///c:/Users/Chetan/Documents/Developer/Projects/Social-Media/feed-service/SYSTEM_DESIGN.md) — Timeline architecture, fanout-on-write vs fanout-on-read, celebrity thresholds, and Redis sorted set indexing.
* 🔔 [**Notification Service System Design**](file:///c:/Users/Chetan/Documents/Developer/Projects/Social-Media/notification-service/SYSTEM_DESIGN.md) — Multi-channel dispatch (In-App WebSockets, FCM/APNs, Email), smart aggregation ("like storms"), and unread badge synchronization.

---

## 🏗️ Architecture Overview

```
                                  ┌────────────────────────┐
                                  │      Client Apps       │
                                  │  (Web / iOS / Android) │
                                  └────┬──────────────┬────┘
                                       │              │
                   GraphQL / gRPC Read │              │ WebSocket (Port 8085)
                                       ▼              ▼
                          ┌─────────────────┐   ┌──────────────────────────────┐
                          │   API Gateway   │   │  Notification WS Gateway     │
                          │   (Port 8080)   │   │  (Persistent Conn Cluster)   │
                          └────────┬────────┘   └──────────────┬───────────────┘
                                   │ gRPC                      │ Redis Pub/Sub
        ┌──────────────────────────┼───────────────────────────┼──────────────────────────┐
        ▼                          ▼                           ▼                          ▼
┌───────────────┐          ┌───────────────┐           ┌───────────────┐          ┌───────────────┐
│ user-service  │          │ post-service  │           │follow-service │          │ feed-service  │
│ (Port 50051)  │          │ (Port 50052)  │           │ (Port 50053)  │          │ (Port 50054)  │
│  PostgreSQL   │          │  PostgreSQL   │           │  PostgreSQL   │          │Redis Timelines│
└───────────────┘          └───────┬───────┘           └───────┬───────┘          └───────▲───────┘
                                   │                           │                          │
                                   │ Domain Events (Outbox)    │ Domain Events (Outbox)   │
                                   ▼                           ▼                          │
                       ┌───────────────────────────────────────────┐                      │
                       │        Redis Streams (social:events)      ├──────────────────────┘
                       └─────────────────────┬─────────────────────┘
                                             │
                                             ▼
                               ┌───────────────────────────┐
                               │   notification-service    │
                               │   (Port 50055 & 8085)     │
                               │  PostgreSQL + Redis WS    │
                               └───────────────────────────┘
```

---

## 📦 Microservices Directory

| Service | Port | Description | Primary Datastore |
| :--- | :--- | :--- | :--- |
| **`api-gateway`** | `:8080` | Public GraphQL API gateway (gqlgen), auth middleware, dataloaders. | In-Memory Query Cache |
| **`user-service`** | `:50051` | User registration, authentication, JWT tokens, profile management. | PostgreSQL |
| **`post-service`** | `:50052` | Post creation, editing, deletion, archiving, media uploads, likes. | PostgreSQL + Cloudinary |
| **`follow-service`** | `:50053` | Social graph, follow/unfollow relationships, follower counts. | PostgreSQL |
| **`feed-service`** | `:50054` | Chronological activity streams, hybrid fanout, Redis timeline cache. | Redis (Sorted Sets) |
| **`notification-service`** | `:50055`, `:8085` | In-app real-time alerts, mobile push (APNs/FCM), email, roll-ups. | PostgreSQL + Redis Pub/Sub |
| **`proto`** | N/A | Shared Protocol Buffer definitions and compiled gRPC code stubs. | N/A |

---

## 🛠️ Local Development & Running Services

### Prerequisites
* Go 1.26+ installed
* PostgreSQL 16+ running on `localhost:5432`
* Redis 7+ running on `localhost:6379`
* [Air](https://github.com/air-verse/air) for live reloading

### Quickstart
1. **Clone the repository and inspect the workspace**:
   ```bash
   git clone https://github.com/chetansachan/socialflow.git
   cd socialflow
   ```
2. **Synchronize dependencies**:
   ```bash
   go work sync
   ```
3. **Start all services with live reload**:
   Using PowerShell:
   ```powershell
   ./dev.ps1
   ```
   Or using Foreman/Honcho with the included `Procfile`:
   ```bash
   foreman start
   ```
4. **Open GraphQL Playground**:
   Navigate to [http://localhost:8080/](http://localhost:8080/) in your browser.

