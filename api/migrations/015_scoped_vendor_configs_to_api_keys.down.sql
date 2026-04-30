DROP INDEX IF EXISTS uniq_vendor_configs_per_client;
DROP INDEX IF EXISTS uniq_vendor_configs_global_vendor_type;
DROP INDEX IF EXISTS idx_vendor_configs_api_key_id;

ALTER TABLE vendor_configs
    DROP COLUMN IF EXISTS api_key_id;

-- Restore original unique constraint on vendor_type
ALTER TABLE vendor_configs
    ADD CONSTRAINT vendor_configs_vendor_type_key UNIQUE (vendor_type);

