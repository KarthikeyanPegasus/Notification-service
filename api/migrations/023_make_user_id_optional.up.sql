-- Make user_id optional in notifications and scheduled_notifications
ALTER TABLE notifications ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE scheduled_notifications ALTER COLUMN user_id DROP NOT NULL;
