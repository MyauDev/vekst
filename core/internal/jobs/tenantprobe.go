package jobs

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
)

// TenantProbeArgs is to tenancy what NoopArgs is to River: a job whose only
// purpose is to prove the wiring end to end. NoopArgs proves that a job
// enqueued in a transaction runs; this one proves that a job which needs a
// tenant gets exactly the one its arguments name, and that a job which names
// none does not run at all.
//
// The organisation arrives by embedding db.TenantJobArgs. It is declared in
// db rather than here because jobs already depends on db and the dependency
// runs one way -- a worker embeds it in its own argument struct rather than db
// learning anything about River.
//
// **Job arguments are not tenant-isolated.** River's tables carry no org_id,
// no row-level-security policy, and are allowlisted out of the coverage test
// (design D4/D5), so every argument struct in this package is readable by
// anything that can read river_job. They therefore carry identifiers and
// never customer financial data: an amount, a counterparty, a description or
// a file's contents in a job payload is data sitting outside the one
// mechanism that keeps tenants apart. Pass the org and the row id; let the
// worker read the row under its own tenant context.
type TenantProbeArgs struct {
	db.TenantJobArgs
}

// Kind satisfies river.JobArgs.
func (TenantProbeArgs) Kind() string { return "tenant_probe" }

// TenantProbeResult is what the worker saw under its tenant context.
type TenantProbeResult struct {
	OrgID    uuid.UUID
	Entities int
}

// tenantProbeObserved, when non-nil, receives each probe's result.
//
// It is unexported and nil in production: nothing outside this package can
// see it, let alone set it. Tests in this package set it to observe what a
// worker actually read, which is the only way to assert "it saw A's rows and
// none of B's" from outside the transaction that read them.
var tenantProbeObserved chan TenantProbeResult

// TenantProbeWorker executes TenantProbeArgs.
//
// It is the shape every future worker copies: take the organisation from the
// job's own arguments, never from ambient state, and reach application data
// only through db.InTx. There is no code path by which a worker can acquire a
// tenant context other than the one written in its own arguments -- no
// package-level current org, no context value, nothing inherited from
// whatever enqueued it.
type TenantProbeWorker struct {
	river.WorkerDefaults[TenantProbeArgs]
	database *db.DB
}

// Work reads the tenant's own entities and nothing else.
//
// OrgIDFromJobArgs is the only door here, and it refuses arguments carrying no
// organisation -- so a job enqueued without one fails loudly and is retried by
// River, rather than running with no tenant context. That matters because "no
// tenant context" would not silently read everything: app_current_org() raises
// (migration 00004). But it would raise from somewhere further in, as a
// database error rather than as a job that was never valid.
func (w *TenantProbeWorker) Work(ctx context.Context, job *river.Job[TenantProbeArgs]) error {
	org, err := db.OrgIDFromJobArgs(job.Args.TenantJobArgs)
	if err != nil {
		return fmt.Errorf("jobs: tenant_probe: %w", err)
	}

	return w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		entities, err := gendb.New(tx).ListEntities(ctx)
		if err != nil {
			return fmt.Errorf("jobs: tenant_probe: listing entities: %w", err)
		}
		if tenantProbeObserved != nil {
			tenantProbeObserved <- TenantProbeResult{
				OrgID:    org.UUID(),
				Entities: len(entities),
			}
		}
		return nil
	})
}
