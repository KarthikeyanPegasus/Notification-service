-- Revert vendor_type length expansion (will fail if longer values exist).

ALTER TABLE vendor_configs
    ALTER COLUMN vendor_type TYPE VARCHAR(20);

