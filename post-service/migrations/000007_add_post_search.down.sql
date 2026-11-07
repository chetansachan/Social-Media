DROP INDEX IF EXISTS idx_posts_content_trgm;
DROP INDEX IF EXISTS idx_posts_hashtags;
DROP INDEX IF EXISTS idx_posts_search_vector;

ALTER TABLE posts DROP COLUMN IF EXISTS search_vector;
ALTER TABLE posts DROP COLUMN IF EXISTS hashtags;

-- pg_trgm is shared with other schemas, so it is intentionally left installed.
