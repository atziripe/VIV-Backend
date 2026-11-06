package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.IneligibilityEventRecorder = (*NeonIneligibilityEventRecorder)(nil)

// NeonIneligibilityEventRecorder is the sole implementation of
// IneligibilityEventRecorder — Neon-only, no Firestore counterpart, since
// there's no per-user document to dual-write against (see
// ReportIneligibilityUseCase).
type NeonIneligibilityEventRecorder struct {
	pool *pgxpool.Pool
}

func NewNeonIneligibilityEventRecorder(pool *pgxpool.Pool) *NeonIneligibilityEventRecorder {
	return &NeonIneligibilityEventRecorder{pool: pool}
}

func (r *NeonIneligibilityEventRecorder) RecordIneligibility(ctx context.Context, reason domain.IneligibilityReason) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ineligibility_events (reason) VALUES ($1)
	`, string(reason))
	return err
}
