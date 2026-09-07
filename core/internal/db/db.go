// Package db owns the single connection pool to Postgres and the single
// entry point every caller -- Connect RPC handler and River worker alike --
// uses to reach it. No other package may hold a *pgxpool.Pool or call
// pool.Query / pool.Begin directly; a CI check (make lint) fails a pull
// request that adds a second door. See openspec design D2.
//
// The pool always connects as vekst_app. vekst_migrator credentials are used
// only by the migration Job and are never present in this process's
// environment (design Q1/Q2).
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

// DB owns the pool and is the only thing in the codebase that may begin a
// transaction.
type DB struct {
	pool *pgxpool.Pool
}

// Config is the subset of core's configuration this package needs. Kept
// separate from core/internal/config's exported type so this package does
// not import it back -- the dependency runs one way.
type Config struct {
	// URL is a libpq connection string, e.g.
	// "postgres://vekst_app:...@host:5432/vekst?sslmode=disable".
	URL string

	// MaxConns bounds the pool. Zero uses pgxpool's own default.
	MaxConns int32

	// ConnectTimeout bounds the initial connection and the startup ping.
	ConnectTimeout time.Duration
}

// New parses cfg and establishes the pool, verifying connectivity with a
// bounded ping before returning. It performs I/O.
func New(ctx context.Context, cfg Config) (*DB, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("db: URL is empty")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("db: parsing connection string: %w", err)
	}
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout(cfg.ConnectTimeout))
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("db: opening pool: %w", err)
	}
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Close releases every connection in the pool. Safe to call once, on
// shutdown.
func (d *DB) Close() {
	d.pool.Close()
}

// Pool exposes the raw pool to River's own driver (core/internal/jobs),
// which needs it for background polling and leader election -- work that
// touches none of River's tables' rows through InSystemTx because River's tables
// carry no tenant data to protect (design D4). This is the one sanctioned
// second consumer of the pool; scripts/check-db-entry-point.sh allows it
// explicitly. Everything that touches application data still goes through
// InSystemTx alone.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

// Ping is used by /readyz (design D6) to check the pool answers, with the
// caller supplying a short timeout.
func (d *DB) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

// AppliedMigrationVersion is used by /readyz to compare the schema version
// goose has applied against the version this binary requires (design D6/Q5).
func (d *DB) AppliedMigrationVersion(ctx context.Context) (int64, error) {
	return gendb.New(d.pool).AppliedMigrationVersion(ctx)
}

func connectTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return 5 * time.Second
	}
	return d
}
