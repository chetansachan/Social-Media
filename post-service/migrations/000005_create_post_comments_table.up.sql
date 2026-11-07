-- Threaded comments. A comment, its parent's reply counter, and the post's
-- comment counter all change inside one transaction so the denormalised counts
-- never drift from the rows they describe.
--
-- parent_id is NULL for a top-level comment. depth makes the nesting limit
-- enforceable in one read instead of walking the parent chain, and lets list
-- queries cap a thread without a recursive CTE.
CREATE TABLE IF NOT EXISTS post_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    author_id UUID NOT NULL,
    parent_id UUID REFERENCES post_comments(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    like_count INT NOT NULL DEFAULT 0,
    reply_count INT NOT NULL DEFAULT 0,
    depth SMALLINT NOT NULL DEFAULT 0,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT post_comments_depth_check CHECK (depth >= 0 AND depth <= 8)
);

-- Top-level comments for a post, newest first. The partial predicate only
-- covers rows the list query can ask for.
CREATE INDEX IF NOT EXISTS idx_post_comments_post_created
    ON post_comments (post_id, created_at DESC, id DESC)
    WHERE parent_id IS NULL;

-- Replies are read oldest first so a conversation reads top to bottom.
CREATE INDEX IF NOT EXISTS idx_post_comments_parent_created
    ON post_comments (parent_id, created_at ASC, id ASC)
    WHERE parent_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_post_comments_author
    ON post_comments (author_id, created_at DESC);

-- comment_count is denormalised on the post so a feed page needs no aggregate
-- query per card.
ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS comment_count INT NOT NULL DEFAULT 0;
