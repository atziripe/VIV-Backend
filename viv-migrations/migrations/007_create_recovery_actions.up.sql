-- Recovery card action log — what the user tapped for a given day's
-- recovery card (done | not_today | acknowledged | trained_anyway).
-- One row per (user, date): natural composite key, no separate generated
-- ID needed — mirrors the Firestore doc-ID-by-date convention this
-- entity already uses (users/{userId}/recovery_actions/{date}).
CREATE TABLE recovery_actions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date        DATE NOT NULL,
    kind        TEXT NOT NULL,
    logged_at   TIMESTAMPTZ NOT NULL,

    UNIQUE (user_id, date)
);

CREATE INDEX idx_recovery_actions_user_date ON recovery_actions(user_id, date);
