package jobs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool skips the test when no live database is configured, rather than
// failing: these exercise real Postgres transaction semantics that a mock
// cannot stand in for. CI's "migrations and roles" job sets DATABASE_URL
// against an already-migrated scratch database.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping a test that needs a live, migrated Postgres")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Task 4.5: a job enqueued in a transaction that commits runs.
func TestInsertTxCommitted(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	client, err := New(pool)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Stop(stopCtx)
	}()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := client.InsertTx(ctx, tx, NoopArgs{}); err != nil {
		t.Fatalf("InsertTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	var count int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM river_job WHERE kind = 'noop' AND state = 'completed'").Scan(&count); err != nil {
			t.Fatalf("querying river_job: %v", err)
		}
		if count > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("a job enqueued in a committed transaction never ran")
}

// Task 4.6: a job enqueued in a transaction that rolls back never exists and
// never runs -- the property that makes ARCHITECTURE.md §4a's atomic import
// possible without a broker (design D3).
func TestInsertTxRolledBack(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	client, err := New(pool)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Compared as a delta, not an absolute count: another test in this
	// package legitimately leaves a completed 'noop' row behind, and river_job
	// carries no per-test identifier to filter by.
	before := countNoopJobs(t, pool, ctx)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := client.InsertTx(ctx, tx, NoopArgs{}); err != nil {
		t.Fatalf("InsertTx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	after := countNoopJobs(t, pool, ctx)
	if after != before {
		t.Fatalf("river_job 'noop' count went from %d to %d after a rolled-back InsertTx, want no change", before, after)
	}
}

func countNoopJobs(t *testing.T, pool *pgxpool.Pool, ctx context.Context) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM river_job WHERE kind = 'noop'").Scan(&count); err != nil {
		t.Fatalf("querying river_job: %v", err)
	}
	return count
}
