-- Messages are append-only apart from a soft-delete tombstone, which keeps a
-- thread's timeline stable for the other side and lets a sender retract without
-- rewriting history.
CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL,
    content VARCHAR(4000) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID
);

-- A thread page is a (conversation_id, created_at DESC, id DESC) scan.
CREATE INDEX IF NOT EXISTS idx_messages_conversation_created
    ON messages (conversation_id, created_at DESC, id DESC);

-- Sender history and "what did this user send?" queries.
CREATE INDEX IF NOT EXISTS idx_messages_sender_created
    ON messages (sender_id, created_at DESC);
