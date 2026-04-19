-- Scheduled notifications: authoritative schedule state + Cadence workflow IDs
CREATE TABLE IF NOT EXISTS scheduled_notifications (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id      UUID NOT NULL UNIQUE REFERENCES notifications(id) ON DELETE CASCADE,
    user_id              UUID NOT NULL,
    channel              VARCHAR(20) NOT NULL,
    template_id          UUID,
    template_vars        JSONB,
    scheduled_at         TIMESTAMPTZ NOT NULL,
    original_at          TIMESTAMPTZ NOT NULL,
    cadence_workflow_id  VARCHAR(256) NOT NULL,
    cadence_run_id       VARCHAR(256) NOT NULL,
    status               VARCHAR(20) NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending','cancelled','running','delivered','failed')),
    reschedule_count     INT NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sched_user   ON scheduled_notifications (user_id, status);
CREATE INDEX IF NOT EXISTS idx_sched_status ON scheduled_notifications (status, scheduled_at);
