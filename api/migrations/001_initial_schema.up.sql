-- ── Consolidated Initial Schema ─────────────────────────────────────────────
-- Replaces migrations 001–035. All statements are idempotent
-- (IF NOT EXISTS / IF NOT NULL / DROP ... IF EXISTS) so this file is safe
-- on both fresh databases and databases already at version 35.

-- ═══════════════════════════════════════════════════════════════════════════
-- Core tables
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS notifications (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key  VARCHAR(128) UNIQUE NOT NULL,
    user_id          UUID,
    channel          VARCHAR(20) NOT NULL CHECK (channel IN ('email','sms','push','websocket','webhook','slack')),
    priority         VARCHAR(10) NOT NULL CHECK (priority IN ('high','medium','low')),
    type             VARCHAR(50) NOT NULL,
    template_id      UUID,
    rendered_content JSONB,
    recipient        VARCHAR(512) NOT NULL,
    status           VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','queued','sent','delivered','failed','cancelled','bounced','suppressed')),
    scheduled_at     TIMESTAMPTZ,
    sent_at          TIMESTAMPTZ,
    delivered_at     TIMESTAMPTZ,
    source           VARCHAR(50) DEFAULT 'unknown',
    api_key_id       UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_created  ON notifications (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_status_updated ON notifications (status, updated_at);
CREATE INDEX IF NOT EXISTS idx_notifications_idempotency    ON notifications (idempotency_key);
CREATE INDEX IF NOT EXISTS idx_notifications_scheduled      ON notifications (scheduled_at) WHERE scheduled_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_source         ON notifications (source);
CREATE INDEX IF NOT EXISTS idx_notifications_api_key_id     ON notifications (api_key_id);

-- ═══════════════════════════════════════════════════════════════════════════

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
    retry_count      INTEGER NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE IF NOT EXISTS notification_attempts_default PARTITION OF notification_attempts DEFAULT;

CREATE INDEX IF NOT EXISTS idx_attempts_notification ON notification_attempts (notification_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_attempts_status       ON notification_attempts (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_attempts_provider     ON notification_attempts (provider, created_at DESC);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS notification_events (
    id               UUID NOT NULL DEFAULT gen_random_uuid(),
    notification_id  UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    event_type       VARCHAR(30) NOT NULL CHECK (event_type IN (
                         'queued','sent','delivered','failed','bounced',
                         'clicked','opened','cancelled',
                         'complained','opted_out','suppressed'
                     )),
    metadata         JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE IF NOT EXISTS notification_events_default PARTITION OF notification_events DEFAULT;

CREATE INDEX IF NOT EXISTS idx_events_notification ON notification_events (notification_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_events_type         ON notification_events (event_type, created_at DESC);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS scheduled_notifications (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id      UUID NOT NULL UNIQUE REFERENCES notifications(id) ON DELETE CASCADE,
    channel              VARCHAR(20) NOT NULL,
    template_id          UUID,
    template_vars        JSONB,
    scheduled_at         TIMESTAMPTZ NOT NULL,
    original_at          TIMESTAMPTZ NOT NULL,
    cadence_workflow_id  VARCHAR(256) NOT NULL,
    cadence_run_id       VARCHAR(256) NOT NULL,
    status               VARCHAR(20) NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending','cancelled','running','delivered','failed')),
    reschedule_count     INT NOT NULL DEFAULT 0,
    api_key_id           UUID,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sched_user   ON scheduled_notifications (status, scheduled_at);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS provider_webhook_events (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider         VARCHAR(50) NOT NULL,
    channel          VARCHAR(20) NOT NULL,
    notification_id  UUID REFERENCES notifications(id) ON DELETE SET NULL,
    event_type       VARCHAR(50) NOT NULL,
    raw_payload      JSONB NOT NULL,
    received_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_provider        ON provider_webhook_events (provider, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_webhook_events_notification    ON provider_webhook_events (notification_id) WHERE notification_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_webhook_events_received        ON provider_webhook_events (received_at DESC);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS notification_templates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) UNIQUE NOT NULL,
    channel     VARCHAR(20) NOT NULL CHECK (channel IN ('email','sms','otp','push','websocket','webhook','slack')),
    subject     VARCHAR(512),
    body        TEXT NOT NULL,
    version     INT NOT NULL DEFAULT 1,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_templates_name    ON notification_templates (name);
CREATE INDEX IF NOT EXISTS idx_templates_channel ON notification_templates (channel, is_active);

-- ═══════════════════════════════════════════════════════════════════════════

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

-- ═══════════════════════════════════════════════════════════════════════════

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

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS vendor_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_type VARCHAR(64) NOT NULL,
    config_json JSONB NOT NULL,
    is_active BOOLEAN DEFAULT true,
    api_key_id UUID,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_vendor_configs_global_vendor_type
    ON vendor_configs (vendor_type) WHERE api_key_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_vendor_configs_per_client
    ON vendor_configs (vendor_type, api_key_id);

CREATE INDEX IF NOT EXISTS idx_vendor_configs_api_key_id
    ON vendor_configs (api_key_id);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    prefix      VARCHAR(16) UNIQUE NOT NULL,
    key_hash    BYTEA NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys (prefix);
CREATE INDEX IF NOT EXISTS idx_api_keys_revoked_at ON api_keys (revoked_at);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email VARCHAR(200) UNIQUE NOT NULL,
  name VARCHAR(100) NOT NULL,
  password_hash BYTEA,
  role VARCHAR(32) NOT NULL CHECK (role IN ('admin','manager','dev','support')),
  clerk_id VARCHAR(255) UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_api_keys (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, api_key_id)
);

CREATE INDEX IF NOT EXISTS idx_user_api_keys_user ON user_api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_user_api_keys_key ON user_api_keys(api_key_id);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS suppressions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type        VARCHAR(20) NOT NULL CHECK (type IN ('email', 'sms', 'push', 'slack', 'webhook')),
    value       VARCHAR(512) NOT NULL,
    reason      VARCHAR(255),
    metadata    JSONB,
    created_by  VARCHAR(255),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_suppressions_type_value ON suppressions (type, value);

CREATE TABLE IF NOT EXISTS opt_outs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL,
    channel     VARCHAR(20) NOT NULL CHECK (channel IN ('email','sms','push','websocket','webhook','slack')),
    reason      VARCHAR(255),
    source      VARCHAR(100),
    created_by  VARCHAR(255),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_opt_outs_user_channel ON opt_outs (user_id, channel);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS vendor_rate_limits (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_name VARCHAR(100) NOT NULL,
    api_key_id  UUID REFERENCES api_keys(id) ON DELETE CASCADE,
    rps         DOUBLE PRECISION,
    per_minute  INTEGER,
    per_10_min  INTEGER,
    per_hour    INTEGER,
    per_day     INTEGER,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX vendor_rate_limits_global_uq
    ON vendor_rate_limits (vendor_name) WHERE api_key_id IS NULL;

CREATE UNIQUE INDEX vendor_rate_limits_scoped_uq
    ON vendor_rate_limits (vendor_name, api_key_id) WHERE api_key_id IS NOT NULL;

CREATE INDEX vendor_rate_limits_vendor_idx ON vendor_rate_limits (vendor_name);
CREATE INDEX vendor_rate_limits_api_key_idx ON vendor_rate_limits (api_key_id);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS orchestration_migrations (
    id UUID PRIMARY KEY,
    api_key_id UUID REFERENCES api_keys(id) ON DELETE CASCADE,
    client_name VARCHAR(255) NOT NULL DEFAULT '',
    old_provider VARCHAR(32) NOT NULL,
    new_provider VARCHAR(32) NOT NULL,
    old_config_json JSONB,
    new_config_json JSONB,
    status VARCHAR(20) NOT NULL DEFAULT 'in_progress',
    old_workflow_count INT NOT NULL DEFAULT 0,
    completed_old_workflows INT NOT NULL DEFAULT 0,
    migrated_scheduled_count INT NOT NULL DEFAULT 0,
    total_scheduled_count INT NOT NULL DEFAULT 0,
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    notified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_om_api_key_id ON orchestration_migrations(api_key_id);
CREATE INDEX IF NOT EXISTS idx_om_status ON orchestration_migrations(status);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS vendor_migrations (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    api_key_id       UUID         REFERENCES api_keys(id) ON DELETE CASCADE,
    channel          VARCHAR(20)  NOT NULL,
    from_vendor      VARCHAR(50)  NOT NULL,
    to_vendor        VARCHAR(50)  NOT NULL,
    from_config_json JSONB,
    to_config_json   JSONB        NOT NULL,
    strategy         VARCHAR(20)  NOT NULL DEFAULT 'instant',
    status           VARCHAR(20)  NOT NULL DEFAULT 'in_progress',
    traffic_percent  INTEGER      NOT NULL DEFAULT 100,
    error_message    TEXT,
    started_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vendor_migrations_api_key ON vendor_migrations(api_key_id);
CREATE INDEX IF NOT EXISTS idx_vendor_migrations_status  ON vendor_migrations(status);
CREATE INDEX IF NOT EXISTS idx_vendor_migrations_channel ON vendor_migrations(channel);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS user_preferences (
    user_id         VARCHAR(255) PRIMARY KEY,
    preferences     JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_preferences_updated_at ON user_preferences (updated_at);

-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS dlq_entries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id UUID,
    channel         VARCHAR(20) NOT NULL,
    recipient       VARCHAR(512),
    reason          TEXT NOT NULL,
    payload         JSONB,
    attempt_count   INT NOT NULL DEFAULT 0,
    replayed        BOOLEAN NOT NULL DEFAULT FALSE,
    replayed_at     TIMESTAMPTZ,
    api_key_id      UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dlq_entries_created_at ON dlq_entries (created_at DESC);
CREATE INDEX idx_dlq_entries_replayed ON dlq_entries (replayed) WHERE replayed = FALSE;
CREATE INDEX idx_dlq_entries_notification_id ON dlq_entries (notification_id);
CREATE INDEX IF NOT EXISTS idx_dlq_entries_api_key_id ON dlq_entries (api_key_id);

-- ═══════════════════════════════════════════════════════════════════════════
-- Functions and triggers
-- ═══════════════════════════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION set_vendor_migration_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_user_preferences_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS update_vendor_configs_updated_at ON vendor_configs;
CREATE TRIGGER update_vendor_configs_updated_at
    BEFORE UPDATE ON vendor_configs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS trg_vendor_migrations_updated_at ON vendor_migrations;
CREATE TRIGGER trg_vendor_migrations_updated_at
    BEFORE UPDATE ON vendor_migrations
    FOR EACH ROW EXECUTE FUNCTION set_vendor_migration_updated_at();

DROP TRIGGER IF EXISTS trg_user_preferences_updated_at ON user_preferences;
CREATE TRIGGER trg_user_preferences_updated_at
    BEFORE UPDATE ON user_preferences
    FOR EACH ROW EXECUTE FUNCTION update_user_preferences_timestamp();

-- ═══════════════════════════════════════════════════════════════════════════
-- Comments
-- ═══════════════════════════════════════════════════════════════════════════

COMMENT ON TABLE dlq_entries IS 'Dead-letter queue entries for notifications that exceeded max retry attempts';
COMMENT ON COLUMN dlq_entries.reason IS 'Reason for DLQ (e.g. max_retries_exceeded, poison_pill, provider_error)';
COMMENT ON COLUMN dlq_entries.replayed IS 'Whether this entry has been replayed';
