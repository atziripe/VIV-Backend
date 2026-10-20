package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewNeonPool creates a pgxpool connection to Neon.
// DATABASE_URL format: postgresql://user:pass@host/dbname?sslmode=require
func NewNeonPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("neon: parse config: %w", err)
	}

	cfg.MaxConns = 10
	cfg.MinConns = 2

	// Neon's connection string routes through PgBouncer in transaction
	// pooling mode, which can hand two different logical connections the
	// same physical backend between transactions. pgx's default exec mode
	// names and caches prepared statements server-side, so two
	// connections that land on the same recycled backend can collide on
	// the same statement name ("prepared statement ... already exists").
	// QueryExecModeExec still uses the extended protocol (correct
	// parameter typing) but never names or caches the statement — the
	// mode pgx's own docs recommend for a transaction-mode pooler.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("neon: connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("neon: ping: %w", err)
	}

	return pool, nil
}
