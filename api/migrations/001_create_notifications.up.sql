-- Notifications: one row per notification request
CREATE TABLE IF NOT EXISTS notifications (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key  VARCHAR(128) UNIQUE NOT NULL,
    user_id          UUID NOT NULL,
    channel          VARCHAR(20) NOT NULL CHECK (channel IN ('email','sms','otp','push','websocket','webhook')),
    priority         VARCHAR(10) NOT NULL CHECK (priority IN ('high','medium','low')),
    type             VARCHAR(50) NOT NULL,
    template_id      UUID,
    rendered_content JSONB,
    recipient        VARCHAR(512) NOT NULL,
    status           VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','queued','sent','delivered','failed','cancelled','bounced')),
    scheduled_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_created  ON notifications (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_status_updated ON notifications (status, updated_at);
CREATE INDEX IF NOT EXISTS idx_notifications_idempotency    ON notifications (idempotency_key);
CREATE INDEX IF NOT EXISTS idx_notifications_scheduled      ON notifications (scheduled_at) WHERE scheduled_at IS NOT NULL;
