-- Mutes hide an author's content from one viewer without telling the author.
-- Like blocks they are directional, but unlike blocks they do not hide the
-- muter's content from the muted user.
CREATE TABLE IF NOT EXISTS user_mutes (
    muter_id UUID NOT NULL,
    muted_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (muter_id, muted_id),
    CONSTRAINT user_mutes_no_self CHECK (muter_id <> muted_id)
);
