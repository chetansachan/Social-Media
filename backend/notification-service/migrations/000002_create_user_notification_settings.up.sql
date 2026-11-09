CREATE TABLE IF NOT EXISTS user_notification_settings (
    user_id UUID PRIMARY KEY,

    -- In-app category toggles.
    inapp_likes BOOLEAN NOT NULL DEFAULT TRUE,
    inapp_comments BOOLEAN NOT NULL DEFAULT TRUE,
    inapp_follows BOOLEAN NOT NULL DEFAULT TRUE,
    inapp_mentions BOOLEAN NOT NULL DEFAULT TRUE,

    -- Push channel and category toggles. Likes default to off to reduce
    -- battery drain and alert fatigue.
    push_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    push_likes BOOLEAN NOT NULL DEFAULT FALSE,
    push_comments BOOLEAN NOT NULL DEFAULT TRUE,
    push_follows BOOLEAN NOT NULL DEFAULT TRUE,
    push_mentions BOOLEAN NOT NULL DEFAULT TRUE,

    -- Email channel.
    email_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    email_weekly_digest BOOLEAN NOT NULL DEFAULT TRUE,
    email_security_alerts BOOLEAN NOT NULL DEFAULT TRUE,

    -- Quiet hours are stored in the user's local timezone.
    quiet_hours_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    quiet_hours_start TIME,
    quiet_hours_end TIME,
    timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
