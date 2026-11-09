-- User autocomplete needs prefix matching ("al" finding "alice") and typo
-- tolerance ("alcie" finding "alice"), which a btree cannot do. pg_trgm
-- covers both; the extra text_pattern_ops index serves the common exact-prefix
-- case without scanning trigrams.
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

CREATE INDEX IF NOT EXISTS idx_users_username_trgm
    ON users USING GIN (lower(username) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_users_name_trgm
    ON users USING GIN (lower(name) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_users_username_prefix
    ON users (lower(username) text_pattern_ops);
