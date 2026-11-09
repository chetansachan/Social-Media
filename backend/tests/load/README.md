# Load tests

[k6](https://k6.io) scenarios for the GraphQL edge. Each script targets one
hot path so a regression points at a single service instead of the whole graph.

## Prerequisites

* k6 0.49 or newer (the WebSocket scenario negotiates a subprotocol).
* The full stack running locally (`./dev.ps1`) or a staging deployment.
* A seeded database: the feed and subscription scenarios assume the user
  follows at least one author who has published posts.
* A valid access token. Register and log in through the gateway, or reuse a
  token from your client:

```bash
curl -s http://localhost:8080/query \
  -H 'Content-Type: application/json' \
  -d '{"query":"mutation($i: LoginInput!){ Login(input:$i){ accessToken } }","variables":{"i":{"email":"load@example.com","password":"Passw0rd!"}}}'
```

The access token expires with the configured TTL. A long run needs a fresh
token; export it immediately before starting k6.

## Scenarios

| Script | Path exercised | Default load | Thresholds |
| :--- | :--- | :--- | :--- |
| `feed_load.js` | `getFeed` + dataloaded authors | ramp to 2,000 VUs over 5 min | p95 < 100 ms, p99 < 250 ms, errors < 1% |
| `post_write_load.js` | `CreatePost` + outbox insert | 50 writes/s for 2 min | p95 < 300 ms, p99 < 600 ms, errors < 1% |
| `login_load.js` | bcrypt + refresh-token row | 20 logins/s for 1 min | p95 < 300 ms, p99 < 750 ms, errors < 1% |
| `ws_notifications.js` | GraphQL subscription over Redis Pub/Sub | ramp to 1,000 subscribers | p95 connect < 1 s, frames > 0 |
| `chat_load.js` | `SendMessage` + inbox + thread reads | 60 msg/s for 2 min, half the VUs reading | p95 < 150 ms, p99 < 400 ms, errors < 1% |
| `search_load.js` | `searchPosts` + `searchUsers` | 60 queries/s for 2 min | p95 < 120 ms, p99 < 300 ms, errors < 1% |

## Environment variables

| Variable | Used by | Meaning |
| :--- | :--- | :--- |
| `GRAPHQL_ENDPOINT` | all HTTP scenarios | Defaults to `http://localhost:8080/query`. |
| `TOKEN` | feed, write, WebSocket | Access token. Mutations fail without it. |
| `AUTHOR_ID` | `post_write_load.js` | Must equal the token owner's id; the gateway returns `FORBIDDEN` on mismatch. |
| `LOAD_EMAIL`, `LOAD_PASSWORD` | `login_load.js` | Credentials of a dedicated load-test account. |
| `WS_URL` | `ws_notifications.js` | Defaults to `ws://localhost:8080/query`. |
| `CONVERSATION_ID` | `chat_load.js` | Thread the token owner belongs to. Required. |
| `TERMS` | `search_load.js` | Comma-separated search terms; defaults to `golang,system,design,postgres`. |
| `HOLD_SECONDS` | `ws_notifications.js` | How long each virtual user keeps a socket open (default 30). |

## Running

```bash
k6 run -e TOKEN="$TOKEN" tests/load/feed_load.js
k6 run -e TOKEN="$TOKEN" -e AUTHOR_ID="$USER_ID" tests/load/post_write_load.js
k6 run -e LOAD_EMAIL="$EMAIL" -e LOAD_PASSWORD="$PASSWORD" tests/load/login_load.js
k6 run -e TOKEN="$TOKEN" tests/load/ws_notifications.js
k6 run -e TOKEN="$TOKEN" -e CONVERSATION_ID="$CONVERSATION_ID" tests/load/chat_load.js
k6 run tests/load/search_load.js
```

To watch a run against a local stack, open the admin metrics while it runs:

```bash
curl -s localhost:9090/metrics | grep -E 'http_request_duration|grpc_server_handling'
```

## Reading the results

* A rising `http_req_duration` p99 while p95 stays flat usually means one
  downstream service is queueing, not that the gateway is saturated.
* `notification_frames_total` at zero with successful upgrades means the
  dispatcher never published: check the `notification_dispatch_total` metric
  and the `notify:<user_id>` Pub/Sub channel.
* `post_write_load.js` commits data. Run it against a disposable database, or
  delete the generated posts afterwards; the content prefix is `load test post`.
* `chat_load.js` commits messages and moves each thread's sort key, so the inbox
  it reads afterwards reflects the other virtual users' writes. Run it against a
  disposable database too.
* Search latency is dominated by the planner. If p99 degrades while write latency
  stays flat, check that `idx_posts_search_vector`, `idx_posts_hashtags`, and the
  trigram indexes still exist and that `pg_trgm` is installed.

## What these do not cover

These are black-box tests of the public edge. They do not replace the unit and
integration suites in each module, and they cannot prove that an outbox event
was published exactly once. Use the metrics and the outbox tables for that.
