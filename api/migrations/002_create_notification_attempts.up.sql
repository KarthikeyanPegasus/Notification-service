-- Notification attempts: one row per provider delivery attempt
CREATE TABLE IF NOT EXISTS notification_attempts (
    id               UUID NOT NULL DEFAULT gen_random_uuid(),
    notification_id  UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    attempt_number   INT NOT NULL DEFAULT 1,
    status           VARCHAR(20) NOT NULL CHECK (status IN ('sent','failed','delivered','bounced')),
    provider         VARCHAR(50) NOT NULL,
    provider_msg_id  VARCHAR(256),
    error_code       VARCHAR(50),
    error_message    TEXT,
    latency_ms       INT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Monthly partitions (pre-create for current and next 3 months)
CREATE TABLE IF NOT EXISTS notification_attempts_default PARTITION OF notification_attempts DEFAULT;

CREATE INDEX IF NOT EXISTS idx_attempts_notification ON notification_attempts (notification_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_attempts_status       ON notification_attempts (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_attempts_provider     ON notification_attempts (provider, created_at DESC);
