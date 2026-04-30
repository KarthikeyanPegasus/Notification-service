ALTER TABLE notification_events DROP CONSTRAINT IF EXISTS notification_events_event_type_check;

ALTER TABLE notification_events
    ADD CONSTRAINT notification_events_event_type_check
    CHECK (event_type IN ('queued','sent','delivered','failed','bounced','clicked','opened','cancelled'));
