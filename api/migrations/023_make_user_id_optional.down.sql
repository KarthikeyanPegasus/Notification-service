-- Revert user_id to NOT NULL
-- WARNING: This may fail if there are rows with NULL user_id
ALTER TABLE notifications ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE scheduled_notifications ALTER COLUMN user_id SET NOT NULL;
