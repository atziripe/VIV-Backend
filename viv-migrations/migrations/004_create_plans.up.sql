-- Generated weekly plans
-- training_json, nutrition_json, recovery_json kept as jsonb —
-- they are LLM-generated blobs with complex nested structure.
-- Can be queried with jsonb operators when needed for analytics.
CREATE TABLE plans (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checkin_id          UUID REFERENCES checkins(id) ON DELETE SET NULL,

    -- Cycle snapshot at generation time
    cycle_phase         TEXT,
    cycle_day           INTEGER,
    cycle_duration      INTEGER,
    cycle_type          TEXT,
    cycle_anchor_at     TIMESTAMPTZ,
    cycle_day_range     TEXT,

    -- Generated content (jsonb for flexibility)
    training_json       JSONB,
    nutrition_json      JSONB,
    recovery_json       JSONB,

    -- Meta
    rest_week_offered   BOOLEAN NOT NULL DEFAULT FALSE,
    plan_version        INTEGER NOT NULL DEFAULT 1,
    status              TEXT NOT NULL DEFAULT 'active',     -- active | archived

    -- Legacy Firestore ID — keep during migration, drop later
    firestore_id        TEXT UNIQUE,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_plans_user_id      ON plans(user_id);
CREATE INDEX idx_plans_checkin_id   ON plans(checkin_id);
CREATE INDEX idx_plans_status       ON plans(status);
CREATE INDEX idx_plans_user_status  ON plans(user_id, status);
CREATE INDEX idx_plans_cycle_phase  ON plans(cycle_phase);
-- GIN index on jsonb for querying inside the plan content
CREATE INDEX idx_plans_training_gin ON plans USING GIN(training_json);

-- Async generation jobs
CREATE TABLE plan_jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checkin_id  UUID REFERENCES checkins(id) ON DELETE SET NULL,
    status      TEXT NOT NULL DEFAULT 'queued',     -- queued | running | done | failed
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_plan_jobs_user_id    ON plan_jobs(user_id);
CREATE INDEX idx_plan_jobs_checkin_id ON plan_jobs(checkin_id);
CREATE INDEX idx_plan_jobs_status     ON plan_jobs(status) WHERE status IN ('queued', 'running');
