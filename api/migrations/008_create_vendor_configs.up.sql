CREATE TABLE vendor_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_type VARCHAR(20) NOT NULL UNIQUE, -- 'sms', 'email', 'push', 'webhook'
    config_json JSONB NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_vendor_configs_updated_at
BEFORE UPDATE ON vendor_configs
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();
