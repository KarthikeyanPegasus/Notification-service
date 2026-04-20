-- Allow slack on templates and notification rows (CHECK constraints from 006 / 011).

-- notification_templates: drop existing channel CHECK (name may vary by Postgres version)
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class t ON c.conrelid = t.oid
        JOIN pg_namespace n ON n.oid = t.relnamespace
        WHERE n.nspname = 'public'
          AND t.relname = 'notification_templates'
          AND c.contype = 'c'
          AND pg_get_constraintdef(c.oid) LIKE '%channel%'
    LOOP
        EXECUTE format('ALTER TABLE notification_templates DROP CONSTRAINT %I', r.conname);
    END LOOP;
END $$;

ALTER TABLE notification_templates
    ADD CONSTRAINT notification_templates_channel_check
    CHECK (channel IN ('email', 'sms', 'otp', 'push', 'websocket', 'webhook', 'slack'));

-- notifications
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_channel_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_channel_check
    CHECK (channel IN ('email', 'sms', 'push', 'websocket', 'webhook', 'slack'));
