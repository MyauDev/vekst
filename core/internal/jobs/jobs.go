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
// package-level global. db.OrgIDFromJobArgs is the only door that turns those
// arguments into a tenant identifier, db.InTx is the only thing that sets the
// context, and TenantProbeWorker is the worked example both are asserted
// against.
//
// The corollary is the one every future worker has to hold on to: **job
// arguments are not tenant-isolated.** River's tables have no org_id and no
// policy, so anything able to read river_job reads every tenant's payloads.
// Arguments therefore carry identifiers -- an organisation, a row id -- and
// never customer financial data. The worker reads the row itself, under its
// own tenant context, where the policy applies.
//
// Every worker reaches application data only through core/internal/db's one
// transaction entry point, never against the pool directly -- the same
// one-door rule design D2 applies to handlers. This package is the one sanctioned exception to "only
// core/internal/db imports pgxpool" (scripts/check-db-entry-point.sh allows
// it explicitly): River's own driver needs the raw pool for its background
// polling and leader election, which touches only River's own
// infrastructure tables -- tables design D4 establishes carry no tenant data
// in the first place, so there is nothing here for a tenant context to
// protect. Enqueuing a job (InsertTx, below) still requires a caller-supplied
// transaction obtained from that entry point, same as any other write.
package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/MyauDev/vekst/core/internal/db"
)

// Client wraps River's client. Its zero value is not usable; construct one
// with New.
type Client struct {
	river *river.Client[pgx.Tx]
}

// New builds a River client bound to pool, with every worker this change
// registers. It performs no I/O; call Start to begin processing.
// WorkerRegistrar lets a package own its own workers and schedules without
// this one importing it. A registrar adds its workers to the registry and
// returns any periodic schedules they need.
//
// It exists so the queries a worker runs stay inside the package that owns
// them -- core/internal/identity's four tables are outside row-level security
// and are read only through that package, a boundary
// scripts/check-identity-queries.sh enforces.
type WorkerRegistrar interface {
	Register(*river.Workers) []*river.PeriodicJob
}

// New takes the *db.DB rather than a bare pool so that workers can reach
// application data through the one transaction entry point, and River's own
// driver can still have the raw pool it needs for polling and leader election
// -- two different needs that used to be served by passing only the second.
func New(database *db.DB, registrars ...WorkerRegistrar) (*Client, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &NoopWorker{})
	river.AddWorker(workers, &TenantProbeWorker{database: database})

	var periodic []*river.PeriodicJob
	for _, r := range registrars {
		periodic = append(periodic, r.Register(workers)...)
	}

	c, err := river.NewClient(riverpgxv5.New(database.Pool()), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers:      workers,
		PeriodicJobs: periodic,
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
