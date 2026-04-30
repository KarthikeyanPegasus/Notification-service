-- Extend governance tables to cover all notification channels.

-- 1. notifications: add 'suppressed' to the status CHECK constraint
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
          AND t.relname = 'notifications'
          AND c.contype = 'c'
          AND pg_get_constraintdef(c.oid) LIKE '%status%'
    LOOP
        EXECUTE format('ALTER TABLE notifications DROP CONSTRAINT %I', r.conname);
    END LOOP;
END $$;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_status_check
    CHECK (status IN ('pending','queued','sent','delivered','failed','cancelled','bounced','suppressed'));

-- 2. suppressions: extend type CHECK to include push/slack/webhook
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
          AND t.relname = 'suppressions'
          AND c.contype = 'c'
          AND pg_get_constraintdef(c.oid) LIKE '%type%'
    LOOP
        EXECUTE format('ALTER TABLE suppressions DROP CONSTRAINT %I', r.conname);
    END LOOP;
END $$;

ALTER TABLE suppressions
    ADD CONSTRAINT suppressions_type_check
    CHECK (type IN ('email', 'sms', 'push', 'slack', 'webhook'));

-- 3. opt_outs: extend channel CHECK to include all channels
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
          AND t.relname = 'opt_outs'
          AND c.contype = 'c'
          AND pg_get_constraintdef(c.oid) LIKE '%channel%'
    LOOP
        EXECUTE format('ALTER TABLE opt_outs DROP CONSTRAINT %I', r.conname);
    END LOOP;
END $$;

ALTER TABLE opt_outs
    ADD CONSTRAINT opt_outs_channel_check
    CHECK (channel IN ('email', 'sms', 'push', 'websocket', 'webhook', 'slack'));
