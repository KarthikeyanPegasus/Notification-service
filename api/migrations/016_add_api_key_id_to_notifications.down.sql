DROP INDEX IF EXISTS idx_notifications_api_key_id;

ALTER TABLE notifications
    DROP COLUMN IF EXISTS api_key_id;

