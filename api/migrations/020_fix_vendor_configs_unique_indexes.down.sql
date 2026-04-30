-- Revert to a single global uniqueness (legacy behavior).
-- NOTE: This will prevent per-client vendor configs from coexisting with globals.

DROP INDEX IF EXISTS uniq_vendor_configs_per_client;
DROP INDEX IF EXISTS uniq_vendor_configs_global_vendor_type;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_vendor_configs_global_vendor_type
    ON vendor_configs (vendor_type);

