-- Lifestyle change reports (illness | travel | injury | sleep_disruption | other).
-- ID is app-generated (uuid.NewString(), see ReportLifestyleChangeUseCase)
-- before the repository ever sees it — firestore_id is that same value,
-- named to match the bridge-column convention checkins/plans already use.
CREATE TABLE lifestyle_changes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    firestore_id    TEXT UNIQUE,

    type            TEXT NOT NULL,
    space_train     TEXT,
    possible_diet   TEXT,
    energy          TEXT,
    applies_to      DATE,
    plan_id         TEXT,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_lifestyle_changes_user_id      ON lifestyle_changes(user_id);
CREATE INDEX idx_lifestyle_changes_user_created ON lifestyle_changes(user_id, created_at DESC);
