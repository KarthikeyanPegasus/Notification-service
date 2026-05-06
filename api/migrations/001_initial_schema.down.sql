-- ── Consolidated Down Migration ──────────────────────────────────────────────
-- Drops all objects created by 001_initial_schema.up.sql, in reverse dependency order.

DROP TRIGGER IF EXISTS trg_user_preferences_updated_at ON user_preferences;
DROP TRIGGER IF EXISTS trg_vendor_migrations_updated_at ON vendor_migrations;
DROP TRIGGER IF EXISTS update_vendor_configs_updated_at ON vendor_configs;

DROP FUNCTION IF EXISTS update_user_preferences_timestamp();
DROP FUNCTION IF EXISTS set_vendor_migration_updated_at();
DROP FUNCTION IF EXISTS update_updated_at_column();

DROP TABLE IF EXISTS dlq_entries;
DROP TABLE IF EXISTS user_preferences;
DROP TABLE IF EXISTS vendor_migrations;
DROP TABLE IF EXISTS orchestration_migrations;
DROP TABLE IF EXISTS vendor_rate_limits;
DROP TABLE IF EXISTS opt_outs;
DROP TABLE IF EXISTS suppressions;
DROP TABLE IF EXISTS user_api_keys;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS vendor_configs;
DROP TABLE IF EXISTS reporting_daily_channel_metrics;
DROP TABLE IF EXISTS device_tokens;
DROP TABLE IF EXISTS notification_templates;
DROP TABLE IF EXISTS provider_webhook_events;
DROP TABLE IF EXISTS scheduled_notifications;
DROP TABLE IF EXISTS notification_events;
DROP TABLE IF EXISTS notification_attempts;
DROP TABLE IF EXISTS notification_attempts_default;
DROP TABLE IF EXISTS notification_events_default;
DROP TABLE IF EXISTS notifications;
