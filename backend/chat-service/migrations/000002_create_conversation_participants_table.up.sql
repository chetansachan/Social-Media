-- Membership is the authorization surface: every read and write resolves
-- against this table before a message or thread is touched.
CREATE TABLE IF NOT EXISTS conversation_participants (
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Read watermark for this member. NULL means "nothing read yet", which is
    -- what makes a new thread show every message as unread.
    last_read_at TIMESTAMPTZ,
    PRIMARY KEY (conversation_id, user_id)
);

-- "Which threads am I in?" runs for every inbox load.
CREATE INDEX IF NOT EXISTS idx_conversation_participants_user
    ON conversation_participants (user_id, joined_at DESC);

-- Unread badges count messages newer than the member's watermark.
CREATE INDEX IF NOT EXISTS idx_conversation_participants_last_read
    ON conversation_participants (user_id, last_read_at);
