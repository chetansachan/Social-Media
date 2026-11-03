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

`notification-service` also reads `DB_CONN_MAX_LIFETIME` (default `5m`). Keep
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
