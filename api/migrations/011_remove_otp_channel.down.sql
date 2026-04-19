-- 1. Drop the new check constraint
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_channel_check;

-- 2. Add the old check constraint including 'otp'
ALTER TABLE notifications ADD CONSTRAINT notifications_channel_check CHECK (channel IN ('email','sms','otp','push','websocket','webhook'));
