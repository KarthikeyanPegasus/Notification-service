-- Up migration for Governance module

-- 1. Suppressions table (Identifier-level blocks)
CREATE TABLE IF NOT EXISTS suppressions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type        VARCHAR(20) NOT NULL CHECK (type IN ('email', 'sms')),
    value       VARCHAR(512) NOT NULL,
    reason      VARCHAR(255),
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_suppressions_type_value ON suppressions (type, value);

-- 2. Opt-outs table (User+Channel level blocks)
CREATE TABLE IF NOT EXISTS opt_outs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL,
    channel     VARCHAR(20) NOT NULL CHECK (channel IN ('email','sms','push')),
    reason      VARCHAR(255),
    source      VARCHAR(100), -- e.g. 'manual', 'unsubscribe_link', 'sms_stop'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_opt_outs_user_channel ON opt_outs (user_id, channel);
