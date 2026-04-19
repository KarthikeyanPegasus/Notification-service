-- Notification events: immutable lifecycle timeline
CREATE TABLE IF NOT EXISTS notification_events (
    id               UUID NOT NULL DEFAULT gen_random_uuid(),
    notification_id  UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    event_type       VARCHAR(30) NOT NULL CHECK (event_type IN ('queued','sent','delivered','failed','bounced','clicked','opened','cancelled')),
    metadata         JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE IF NOT EXISTS notification_events_default PARTITION OF notification_events DEFAULT;

CREATE INDEX IF NOT EXISTS idx_events_notification ON notification_events (notification_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_events_type         ON notification_events (event_type, created_at DESC);
