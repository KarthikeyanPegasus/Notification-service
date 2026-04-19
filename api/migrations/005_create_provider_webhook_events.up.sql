-- Provider webhook events: raw inbound callbacks from email/SMS/push providers
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
