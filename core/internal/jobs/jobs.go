// Package jobs owns River: the client, the worker registry, and their
// lifecycle. Change 0.2 registers one no-op job to prove the wiring end to
// end -- a real job arrives with ingest (2.2) and classification (3.2). See
// openspec design D3.
//
// River's own tables carry no org_id and no row-level-security policy: they
// are infrastructure, not tenant data (design D4). A worker therefore takes
// its tenant identifier from its own job arguments and never from ambient
// state -- a job payload is untrusted input for tenancy purposes, the same
// way an HTTP handler trusts only the authenticated session and never a
// package-level global. Change 1.1 adds the `SET LOCAL app.org_id` call
// inside core/internal/db.InTx that a worker's fn must invoke from those
// arguments; there is no tenant concept in this change's data model yet for
// that rule to be enforced against, so today it is documented here, not
// guarded by a test.
//
// Every worker reaches application data only through core/internal/db.InTx,
// never against the pool directly -- the same one-door rule design D2
// applies to handlers. This package is the one sanctioned exception to "only
// core/internal/db imports pgxpool" (scripts/check-db-entry-point.sh allows
// it explicitly): River's own driver needs the raw pool for its background
// polling and leader election, which touches only River's own
// infrastructure tables -- tables design D4 establishes carry no tenant data
// in the first place, so there is nothing here for InTx's tenant context to
// protect. Enqueuing a job (InsertTx, below) still requires a caller-supplied
// transaction obtained from InTx, same as any other write.
package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// Client wraps River's client. Its zero value is not usable; construct one
// with New.
type Client struct {
	river *river.Client[pgx.Tx]
}

// New builds a River client bound to pool, with every worker this change
// registers. It performs no I/O; call Start to begin processing.
func New(pool *pgxpool.Pool) (*Client, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &NoopWorker{})

	c, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: constructing river client: %w", err)
	}
	return &Client{river: c}, nil
}

// Start begins processing jobs. Call it from the server's startup path,
// after the pool is established.
func (c *Client) Start(ctx context.Context) error {
	if err := c.river.Start(ctx); err != nil {
		return fmt.Errorf("jobs: starting river client: %w", err)
	}
	return nil
}

// Stop drains in-flight jobs and stops accepting new ones, so a job is never
// left claimed by a worker that no longer exists (design: shutdown drains
// rather than abandons). Call it from the server's shutdown path, bounded by
// the same ShutdownTimeout as the HTTP drain.
func (c *Client) Stop(ctx context.Context) error {
	if err := c.river.Stop(ctx); err != nil {
		return fmt.Errorf("jobs: stopping river client: %w", err)
	}
	return nil
}

// InsertTx enqueues args inside tx. The job exists if and only if tx
// commits -- design D3, exercised by TestInsertTx in noop_test.go.
func (c *Client) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs) error {
	if _, err := c.river.InsertTx(ctx, tx, args, nil); err != nil {
		return fmt.Errorf("jobs: insert: %w", err)
	}
	return nil
}
