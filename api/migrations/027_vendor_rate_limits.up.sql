-- vendor_rate_limits stores per-vendor throughput caps, optionally scoped to an API key.
-- All window limits are optional; only set (non-null) ones are enforced.
-- Workers check every configured window before calling the provider; all windows must pass.

CREATE TABLE IF NOT EXISTS vendor_rate_limits (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_name VARCHAR(100) NOT NULL,
    api_key_id  UUID REFERENCES api_keys(id) ON DELETE CASCADE,
    rps         DOUBLE PRECISION,   -- requests per second (fractional OK, e.g. 0.5)
    per_minute  INTEGER,
    per_10_min  INTEGER,
    per_hour    INTEGER,
    per_day     INTEGER,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique: one global limit per vendor (api_key_id IS NULL)
CREATE UNIQUE INDEX vendor_rate_limits_global_uq
    ON vendor_rate_limits (vendor_name)
    WHERE api_key_id IS NULL;

-- Unique: one scoped limit per vendor per API key
CREATE UNIQUE INDEX vendor_rate_limits_scoped_uq
    ON vendor_rate_limits (vendor_name, api_key_id)
    WHERE api_key_id IS NOT NULL;

CREATE INDEX vendor_rate_limits_vendor_idx ON vendor_rate_limits (vendor_name);
CREATE INDEX vendor_rate_limits_api_key_idx ON vendor_rate_limits (api_key_id);
