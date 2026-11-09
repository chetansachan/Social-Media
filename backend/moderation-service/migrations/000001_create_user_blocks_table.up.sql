-- Directional block graph. A block hides content in both directions, but the
-- row records who asked for it so unblock only affects that person's choice.
CREATE TABLE IF NOT EXISTS user_blocks (
    blocker_id UUID NOT NULL,
    blocked_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (blocker_id, blocked_id),
    CONSTRAINT user_blocks_no_self CHECK (blocker_id <> blocked_id)
);

-- "Did this author block me?" is the reverse-direction lookup the gateway
-- needs when rendering somebody else's content.
CREATE INDEX IF NOT EXISTS idx_user_blocks_blocked ON user_blocks (blocked_id);
