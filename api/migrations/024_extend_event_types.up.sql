-- Extend notification_events.event_type to include complaint, opt-out, and suppressed events.
-- The original CHECK must be dropped (partitioned tables require dropping on the parent) then re-created.
ALTER TABLE notification_events DROP CONSTRAINT IF EXISTS notification_events_event_type_check;

ALTER TABLE notification_events
    ADD CONSTRAINT notification_events_event_type_check
    CHECK (event_type IN (
        'queued','sent','delivered','failed','bounced',
        'clicked','opened','cancelled',
        'complained','opted_out','suppressed'
    ));
