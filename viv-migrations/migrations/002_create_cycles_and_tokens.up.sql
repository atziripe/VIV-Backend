-- Cycle state (updated daily by the cron job)
-- Kept separate from user_profiles because it changes constantly
CREATE TABLE user_cycles (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    cycle_day           INTEGER,
    cycle_duration      INTEGER,
    cycle_phase         TEXT,       -- menstrual | follicular | ovulatory | early_luteal | late_luteal
    cycle_type          TEXT,       -- natural | hormonal | unknown
    cycle_anchor_at     TIMESTAMPTZ,
    period_duration     TEXT,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_cycles_user_id ON user_cycles(user_id);
CREATE INDEX idx_user_cycles_phase   ON user_cycles(cycle_phase);

-- Push notification tokens
CREATE TABLE device_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fcm_token   TEXT NOT NULL UNIQUE,
    platform    TEXT NOT NULL,      -- ios | android
    timezone    TEXT NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_device_tokens_user_id ON device_tokens(user_id);
CREATE INDEX idx_device_tokens_active  ON device_tokens(active) WHERE active = TRUE;
CREATE INDEX idx_device_tokens_tz      ON device_tokens(timezone) WHERE active = TRUE;
