-- Pure aggregate statistic for ReportIneligibilityUseCase — deliberately
-- has no user reference column of any kind, so a row can never be traced
-- back to the person who triggered it. See domain.IneligibilityReason for
-- the closed set of values "reason" holds.
CREATE TABLE ineligibility_events (
    id          BIGSERIAL PRIMARY KEY,
    reason      TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ineligibility_events_reason_date ON ineligibility_events(reason, occurred_at DESC);
