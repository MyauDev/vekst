package jobs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/MyauDev/vekst/core/internal/db"
)

// startProbeClient boots a River client with the probe worker registered and
// an observation channel installed, and returns the client plus that channel.
func startProbeClient(t *testing.T, database *db.DB) (*Client, chan TenantProbeResult) {
	t.Helper()
	observed := make(chan TenantProbeResult, 4)
	tenantProbeObserved = observed
	t.Cleanup(func() { tenantProbeObserved = nil })

	client, err := New(database)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Stop(stopCtx)
	})
	return client, observed
}

// seedOrg creates an organisation with its single entity through the real
// path, and returns its identifier.
func seedOrg(t *testing.T, database *db.DB, name string) uuid.UUID {
	t.Helper()
	var userID uuid.UUID
	err := database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var id [16]byte
		if err := tx.QueryRow(ctx,
			`INSERT INTO users (email) VALUES ($1) RETURNING id`,
			uuid.NewString()+"@tenancy.test").Scan(&id); err != nil {
			return err
		}
		userID = id
		return nil
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}

	org, err := database.CreateOrganization(context.Background(), db.NewOrganization{
		Name:         name,
		Country:      "NO",
		BaseCurrency: "NOK",
		EntityName:   name + " AS",
		CreatorID:    userID,
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	return org.UUID()
}

// Task 4.2. A job whose arguments name org A reads A's rows and none of B's.
//
// Both organisations exist and both have exactly one entity, so a worker that
// leaked across the boundary would see two rather than one -- the count is the
// assertion, not merely that the job succeeded.
func TestJobReadsOnlyItsOwnTenantsRows(t *testing.T) {
	database := testDatabase(t)
	client, observed := startProbeClient(t, database)
	ctx := context.Background()

	orgA := seedOrg(t, database, "Probe A")
	// Org B exists purely to be invisible: it has one entity of its own, so a
	// worker that leaked across the boundary would count two.
	seedOrg(t, database, "Probe B")

	if err := database.InSystemTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return client.InsertTx(ctx, tx, TenantProbeArgs{
			TenantJobArgs: db.TenantJobArgs{OrgID: orgA},
		})
	}); err != nil {
		t.Fatalf("enqueuing the probe: %v", err)
	}

	select {
	case got := <-observed:
		if got.OrgID != orgA {
			t.Errorf("worker ran for org %s, want %s", got.OrgID, orgA)
		}
		if got.Entities != 1 {
			t.Errorf("worker saw %d entities under org A, want 1; org B's rows are visible to it",
				got.Entities)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the probe job did not run within 15s")
	}

	// Nothing else ran: a second result would mean the worker was invoked for
	// an organisation nobody enqueued.
	select {
	case extra := <-observed:
		t.Errorf("a second probe ran unexpectedly, for org %s", extra.OrgID)
	case <-time.After(500 * time.Millisecond):
	}
}

// Task 4.3. A job whose arguments carry no organisation must fail rather than
// run with no tenant context.
//
// This is asserted at the constructor rather than by waiting for River to
// exhaust its retries: OrgIDFromJobArgs is the door, and the worker's first
// act is to go through it. Driving the same job through the queue would prove
// the same thing more slowly and less precisely -- River would report a failed
// job without saying which of several reasons caused it.
func TestJobWithNoOrganisationDoesNotRun(t *testing.T) {
	database := testDatabase(t)

	w := &TenantProbeWorker{database: database}
	err := w.Work(context.Background(), &river.Job[TenantProbeArgs]{
		Args: TenantProbeArgs{}, // no organisation at all
	})
	if err == nil {
		t.Fatal("a worker with no organisation in its arguments ran to completion")
	}
	if !strings.Contains(err.Error(), "organisation") {
		t.Errorf("error %q does not say what is missing", err)
	}

	// And the same worker, given one, does run -- so the failure above is
	// about the missing organisation and not about the worker being broken.
	orgID := seedOrg(t, database, "Probe C")
	observed := make(chan TenantProbeResult, 1)
	tenantProbeObserved = observed
	t.Cleanup(func() { tenantProbeObserved = nil })

	if err := w.Work(context.Background(), &river.Job[TenantProbeArgs]{
		Args: TenantProbeArgs{TenantJobArgs: db.TenantJobArgs{OrgID: orgID}},
	}); err != nil {
		t.Fatalf("worker with a valid organisation: %v", err)
	}
	select {
	case got := <-observed:
		if got.Entities != 1 {
			t.Errorf("worker saw %d entities, want 1", got.Entities)
		}
	default:
		t.Error("the worker did not report a result")
	}
}
