-- Device tokens for push notifications
CREATE TABLE IF NOT EXISTS device_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL,
    token        VARCHAR(512) NOT NULL UNIQUE,
    platform     VARCHAR(10) NOT NULL CHECK (platform IN ('ios','android','web')),
    app_version  VARCHAR(20),
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_device_tokens_user_active ON device_tokens (user_id, is_active);
CREATE INDEX IF NOT EXISTS idx_device_tokens_token       ON device_tokens (token);

-- Daily channel metrics (materialized for reporting)
CREATE TABLE IF NOT EXISTS reporting_daily_channel_metrics (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metric_date     DATE NOT NULL,
    channel         VARCHAR(20) NOT NULL,
    provider        VARCHAR(50) NOT NULL DEFAULT '',
    total_sent      BIGINT NOT NULL DEFAULT 0,
    total_delivered BIGINT NOT NULL DEFAULT 0,
    total_failed    BIGINT NOT NULL DEFAULT 0,
    total_bounced   BIGINT NOT NULL DEFAULT 0,
    avg_latency_ms  NUMERIC(10,2),
    p50_latency_ms  INT,
    p95_latency_ms  INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (metric_date, channel, provider)
);

CREATE INDEX IF NOT EXISTS idx_metrics_date    ON reporting_daily_channel_metrics (metric_date DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_channel ON reporting_daily_channel_metrics (channel, metric_date DESC);
