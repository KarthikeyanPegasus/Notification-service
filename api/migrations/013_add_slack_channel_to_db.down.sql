-- Revert slack channel (fails if any row still uses channel = slack)

ALTER TABLE notification_templates DROP CONSTRAINT IF EXISTS notification_templates_channel_check;
ALTER TABLE notification_templates
    ADD CONSTRAINT notification_templates_channel_check
    CHECK (channel IN ('email', 'sms', 'otp', 'push', 'websocket', 'webhook'));

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_channel_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_channel_check
    CHECK (channel IN ('email', 'sms', 'push', 'websocket', 'webhook'));
