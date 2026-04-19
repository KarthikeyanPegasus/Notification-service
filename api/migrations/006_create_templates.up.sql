-- Notification templates with Handlebars-style variable substitution
CREATE TABLE IF NOT EXISTS notification_templates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) UNIQUE NOT NULL,
    channel     VARCHAR(20) NOT NULL CHECK (channel IN ('email','sms','otp','push','websocket','webhook')),
    subject     VARCHAR(512),
    body        TEXT NOT NULL,
    version     INT NOT NULL DEFAULT 1,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_templates_name    ON notification_templates (name);
CREATE INDEX IF NOT EXISTS idx_templates_channel ON notification_templates (channel, is_active);
