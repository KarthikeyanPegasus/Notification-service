DROP INDEX IF EXISTS idx_notifications_source;
ALTER TABLE notifications DROP COLUMN IF EXISTS source;
