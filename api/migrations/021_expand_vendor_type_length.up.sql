-- Expand vendor_configs.vendor_type to allow longer names (e.g. "workflow_orchestration").
-- Previously it was VARCHAR(20) which breaks new per-client settings stored as vendor configs.

ALTER TABLE vendor_configs
    ALTER COLUMN vendor_type TYPE VARCHAR(64);

