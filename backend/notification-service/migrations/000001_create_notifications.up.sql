CREATE EXTENSION IF NOT EXISTS "pgcrypto";

DO $$
BEGIN
    CREATE TYPE notification_type AS ENUM (
        'follow',
        'post_like',
        'comment',
        'comment_reply',
        'mention',
        'creator_post',
        'system_alert'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- The inbox is range-partitioned by creation month. Partitioning keeps the hot
-- index small and lets retention drop whole months without a vacuum storm.
-- PostgreSQL requires every unique/primary key on a partitioned table to
-- include the partition key, hence the composite primary key.
CREATE TABLE IF NOT EXISTS notifications (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    recipient_id UUID NOT NULL,
    type notification_type NOT NULL,

    -- Target entity being acted upon (post ID, comment ID, user ID...).
    entity_id VARCHAR(64) NOT NULL,
    entity_type VARCHAR(32) NOT NULL,

    -- Up to three most recent actor IDs are retained for rendering
    -- "Alice, Bob, and 45 others". The full count lives in total_actors_count.
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

-- Create the current month and the next two months. The application repeats
-- this idempotently at startup and once per day so a rolling deployment never
-- reaches a month with no partition.
DO $$
DECLARE
    partition_start DATE;
    partition_end DATE;
    partition_name TEXT;
BEGIN
    FOR i IN 0..2 LOOP
        partition_start := (date_trunc('month', now() AT TIME ZONE 'UTC') + (i || ' month')::interval)::date;
        partition_end := (partition_start + interval '1 month')::date;
        partition_name := 'notifications_' || to_char(partition_start, 'YYYY_MM');

        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF notifications FOR VALUES FROM (%L) TO (%L)',
            partition_name, partition_start, partition_end
        );
    END LOOP;
END $$;

-- Inbox pagination: newest first with UUID as a stable tie-breaker.
CREATE INDEX IF NOT EXISTS idx_notifications_recipient_created
    ON notifications (recipient_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

-- Unread badge fallback when the Redis counter is cold.
CREATE INDEX IF NOT EXISTS idx_notifications_recipient_unread
    ON notifications (recipient_id, created_at DESC)
    WHERE is_read = FALSE AND deleted_at IS NULL;

-- Aggregation lookup / future analytics by entity.
CREATE INDEX IF NOT EXISTS idx_notifications_entity_aggregation
    ON notifications (recipient_id, entity_id, type, created_at DESC)
    WHERE deleted_at IS NULL;
