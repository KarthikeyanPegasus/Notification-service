-- Revert governance channel extensions.

-- 1. notifications: revert status check
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_status_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_status_check
    CHECK (status IN ('pending','queued','sent','delivered','failed','cancelled','bounced'));

-- 2. suppressions: revert type check
ALTER TABLE suppressions DROP CONSTRAINT IF EXISTS suppressions_type_check;
ALTER TABLE suppressions
    ADD CONSTRAINT suppressions_type_check
    CHECK (type IN ('email', 'sms'));

-- 3. opt_outs: revert channel check
ALTER TABLE opt_outs DROP CONSTRAINT IF EXISTS opt_outs_channel_check;
ALTER TABLE opt_outs
    ADD CONSTRAINT opt_outs_channel_check
    CHECK (channel IN ('email', 'sms', 'push'));
