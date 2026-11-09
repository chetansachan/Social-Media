CREATE TABLE IF NOT EXISTS user_device_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    token VARCHAR(512) NOT NULL UNIQUE,
    platform VARCHAR(16) NOT NULL CHECK (platform IN ('ios', 'android', 'web')),
    device_name VARCHAR(128),
    app_version VARCHAR(32),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Push workers only ever need active tokens for one user.
CREATE INDEX IF NOT EXISTS idx_device_tokens_user
    ON user_device_tokens (user_id)
    WHERE is_active = TRUE;
