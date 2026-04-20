ALTER TABLE notifications ADD COLUMN source VARCHAR(50) DEFAULT 'unknown';
CREATE INDEX idx_notifications_source ON notifications(source);
