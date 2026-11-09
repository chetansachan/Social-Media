-- Reports feed a human review queue. target_id points at a post, comment, or
-- user depending on target_type; a polymorphic id stays valid across services
-- because every domain object uses a UUID.
CREATE TABLE IF NOT EXISTS content_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id UUID NOT NULL,
    target_type VARCHAR(16) NOT NULL,
    target_id UUID NOT NULL,
    reason VARCHAR(32) NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    reviewer_id UUID,
    review_note TEXT NOT NULL DEFAULT '',
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT content_reports_target_type_check
        CHECK (target_type IN ('POST', 'COMMENT', 'USER')),
    CONSTRAINT content_reports_reason_check
        CHECK (reason IN ('SPAM', 'HARASSMENT', 'HATE', 'VIOLENCE',
                          'NSFW', 'IMPERSONATION', 'SELF_HARM', 'OTHER')),
    CONSTRAINT content_reports_status_check
        CHECK (status IN ('PENDING', 'REVIEWING', 'ACTIONED', 'DISMISSED'))
);

-- One open report per reporter per target. A partial unique index makes the
-- insert idempotent under concurrency, so double-tapping "report" is harmless.
CREATE UNIQUE INDEX IF NOT EXISTS idx_content_reports_open_unique
    ON content_reports (reporter_id, target_type, target_id)
    WHERE status IN ('PENDING', 'REVIEWING');

-- The reviewer queue reads oldest-first within a status, but keyset pagination
-- pages newest-first; this index serves both the filter and the ordering.
CREATE INDEX IF NOT EXISTS idx_content_reports_status_created
    ON content_reports (status, created_at DESC, id DESC);

-- A moderator often opens one target's full report history.
CREATE INDEX IF NOT EXISTS idx_content_reports_target
    ON content_reports (target_type, target_id, created_at DESC);
