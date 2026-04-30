ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS api_key_id UUID NULL REFERENCES api_keys(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_notifications_api_key_id ON notifications(api_key_id);

