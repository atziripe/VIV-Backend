-- Weekly check-ins — the core input for plan generation
-- phase_feedback stored as jsonb: structure varies per phase
CREATE TABLE checkins (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Legacy Firestore ID — keep during migration, drop later
    firestore_id        TEXT UNIQUE,

    -- Check-in dimensions
    readiness           TEXT,       -- "Ready to build" | "Maintaining" | etc.
    demand              TEXT,       -- "Manageable" | "High" | etc.
    sleep_quality       TEXT,
    last_week_feel      TEXT,
    body_status         TEXT,
    appetite            TEXT,
    predictability      TEXT,
    modality_skip       TEXT,       -- "HIIT" | "Strength" | etc.
    pms_symptoms        TEXT,
    additional_notes    TEXT,
    cycle_start         TEXT,

    -- Feedback on phase content (variable structure per phase)
    phase_feedback      JSONB,

    -- Meta
    status              TEXT NOT NULL DEFAULT 'active',     -- active | archived
    plan_version        INTEGER NOT NULL DEFAULT 1,
    source_event_id     TEXT,
    generated_from      TEXT,       -- "gpt-4.1-mini" | etc.

    week_start          TIMESTAMPTZ,
    end_date            TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_checkins_user_id    ON checkins(user_id);
CREATE INDEX idx_checkins_week_start ON checkins(week_start DESC);
CREATE INDEX idx_checkins_status     ON checkins(status);
-- Useful for ML: get all checkins for a user ordered by week
CREATE INDEX idx_checkins_user_week  ON checkins(user_id, week_start DESC);
