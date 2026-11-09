-- Search runs inside post-service next to the rows it ranks, so a search hit
-- is never staler than the post itself. pg_trgm covers partial words that a
-- tsquery cannot match ("gol" finding "golang"); the generated tsvector covers
-- stemmed keyword search.
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Hashtags are extracted by the application on write so a hashtag query is an
-- exact array overlap instead of a scan over post bodies.
ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS hashtags TEXT[] NOT NULL DEFAULT '{}';

-- Generated keeps the document in sync with content on every update without a
-- trigger or a second write path. The literal config is immutable, which is
-- what lets PostgreSQL store the result.
ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;

CREATE INDEX IF NOT EXISTS idx_posts_search_vector
    ON posts USING GIN (search_vector);

CREATE INDEX IF NOT EXISTS idx_posts_hashtags
    ON posts USING GIN (hashtags);

CREATE INDEX IF NOT EXISTS idx_posts_content_trgm
    ON posts USING GIN (content gin_trgm_ops);
