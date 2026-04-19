-- 1. Migrate existing 'otp' records to 'sms'
UPDATE notifications SET channel = 'sms' WHERE channel = 'otp';

-- 2. Drop the old check constraint. We need to find the constraint name first, 
-- but since we are creating it in 001, it usually has a system name or we can just redefine the table.
-- In PostgreSQL, we can search for constraints of a specific type.
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND contype = 'c' AND consrc LIKE '%otp%';
    
    IF constraint_name IS NOT NULL THEN
        EXECUTE 'ALTER TABLE notifications DROP CONSTRAINT ' || constraint_name;
    END IF;
END $$;

-- 3. Add the new check constraint without 'otp'
ALTER TABLE notifications ADD CONSTRAINT notifications_channel_check CHECK (channel IN ('email','sms','push','websocket','webhook'));
