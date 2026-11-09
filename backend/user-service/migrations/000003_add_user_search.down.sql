DROP INDEX IF EXISTS idx_users_username_prefix;
DROP INDEX IF EXISTS idx_users_name_trgm;
DROP INDEX IF EXISTS idx_users_username_trgm;

-- pg_trgm is shared with other schemas, so it is intentionally left installed.
