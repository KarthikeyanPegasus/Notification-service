-- Fix vendor_configs uniqueness so global + per-client rows can coexist.
-- Global row: api_key_id IS NULL => unique on vendor_type for only NULL api_key_id rows.
-- Per-client row: unique on (vendor_type, api_key_id).

-- Drop any global unique index that incorrectly applies to all rows.
DROP INDEX IF EXISTS uniq_vendor_configs_global_vendor_type;

-- Ensure only one global config per vendor_type (NULL api_key_id).
CREATE UNIQUE INDEX IF NOT EXISTS uniq_vendor_configs_global_vendor_type
    ON vendor_configs (vendor_type)
    WHERE api_key_id IS NULL;

-- Ensure a client can have at most one config per vendor_type.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_vendor_configs_per_client
    ON vendor_configs (vendor_type, api_key_id);

