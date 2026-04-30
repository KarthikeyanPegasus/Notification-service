-- Allow per-client (per api_key) vendor configs by scoping vendor_configs rows.
-- Global configs have api_key_id = NULL.

ALTER TABLE vendor_configs
    ADD COLUMN IF NOT EXISTS api_key_id UUID NULL REFERENCES api_keys(id) ON DELETE CASCADE;

-- Drop old uniqueness if present (created in 008_create_vendor_configs)
ALTER TABLE vendor_configs
    DROP CONSTRAINT IF EXISTS vendor_configs_vendor_type_key;

-- Ensure only one global config per vendor_type (api_key_id IS NULL)
CREATE UNIQUE INDEX IF NOT EXISTS uniq_vendor_configs_global_vendor_type
    ON vendor_configs (vendor_type)
    WHERE api_key_id IS NULL;

-- Ensure a client can have at most one config per vendor_type
CREATE UNIQUE INDEX IF NOT EXISTS uniq_vendor_configs_per_client
    ON vendor_configs (vendor_type, api_key_id);

CREATE INDEX IF NOT EXISTS idx_vendor_configs_api_key_id
    ON vendor_configs (api_key_id);

