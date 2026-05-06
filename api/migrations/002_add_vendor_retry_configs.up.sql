-- ── Vendor Retry Configs ─────────────────────────────────────────────────────
-- Per-vendor retry/backoff settings, optionally scoped to a client (api_key_id).
-- When no scoped config exists, workers use defaults (100ms initial, 30s max, 5 attempts, 2.0 coefficient).

CREATE TABLE IF NOT EXISTS vendor_retry_configs (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_name               VARCHAR(100) NOT NULL,
    api_key_id                UUID REFERENCES api_keys(id) ON DELETE CASCADE,
    retry_initial_interval_ms INTEGER NOT NULL DEFAULT 100,
    retry_max_interval_ms     INTEGER NOT NULL DEFAULT 30000,
    retry_max_attempts        INTEGER NOT NULL DEFAULT 5,
    retry_backoff_coefficient REAL NOT NULL DEFAULT 2.0,
    sla_seconds               INTEGER NOT NULL DEFAULT 30,
    is_active                 BOOLEAN NOT NULL DEFAULT true,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique constraint: one config per vendor per client scope
CREATE UNIQUE INDEX IF NOT EXISTS idx_vendor_retry_configs_unique_null
    ON vendor_retry_configs(vendor_name)
    WHERE api_key_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_vendor_retry_configs_unique_scoped
    ON vendor_retry_configs(vendor_name, api_key_id)
    WHERE api_key_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_vendor_retry_configs_scope
    ON vendor_retry_configs(vendor_name, api_key_id);
