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
	//
	// QueryExecModeExec avoids naming/caching the statement, but it also
	// skips the Describe round trip — so pgx no longer learns the real
	// column type (e.g. jsonb) and falls back to inferring it purely
	// from the Go argument's type, sending a []byte as bytea instead of
	// json/jsonb ("invalid input syntax for type json"). DescribeExec
	// still describes each query (so parameter types are correct) while
	// never using a named/cached statement — the mode pgx recommends for
	// a transaction-mode pooler like PgBouncer.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeDescribeExec

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("neon: connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("neon: ping: %w", err)
	}

	return pool, nil
}
