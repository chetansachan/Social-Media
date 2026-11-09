-- A conversation is either a one-on-one thread or a named group. direct_key
-- encodes the two members of a DIRECT thread in a stable order, so a unique
-- index can guarantee that re-opening a chat never creates a second thread even
-- when both sides click "message" at the same moment.
CREATE TABLE IF NOT EXISTS conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(16) NOT NULL DEFAULT 'DIRECT',
    title VARCHAR(100) NOT NULL DEFAULT '',
    created_by UUID NOT NULL,
    direct_key VARCHAR(100),
    last_message_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT conversations_type_check CHECK (type IN ('DIRECT', 'GROUP')),
    CONSTRAINT conversations_direct_type_check
        CHECK ((type = 'DIRECT' AND direct_key IS NOT NULL)
            OR (type = 'GROUP' AND direct_key IS NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_conversations_direct_key
    ON conversations (direct_key)
    WHERE direct_key IS NOT NULL;

-- The inbox sorts by last activity, so that is the lead index.
CREATE INDEX IF NOT EXISTS idx_conversations_last_message
    ON conversations (COALESCE(last_message_at, created_at) DESC, id DESC);
