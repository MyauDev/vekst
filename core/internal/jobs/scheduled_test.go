package jobs

import (
	"context"
	"testing"
	"time"
)

// add-file-upload's expiry job is the first caller that needs a job to run
// later rather than as soon as a worker is free: a batch's signed URL is
// good for UploadURLLifetime, and sweeping it before that would abandon an
// upload still in flight. This asserts the scheduled time actually reaches
// river_job rather than being silently dropped the way passing river.InsertTx
// a nil opts always does.
func TestInsertTxAtSchedulesForTheFuture(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	client, err := New(testDatabase(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	runAt := time.Now().Add(time.Hour).Truncate(time.Millisecond)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := client.InsertTxAt(ctx, tx, NoopArgs{}, runAt); err != nil {
		t.Fatalf("InsertTxAt: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	var state string
	var scheduledAt time.Time
	err = pool.QueryRow(ctx,
		`SELECT state, scheduled_at FROM river_job
		 WHERE kind = 'noop' ORDER BY id DESC LIMIT 1`,
	).Scan(&state, &scheduledAt)
	if err != nil {
		t.Fatalf("querying river_job: %v", err)
	}

	if state != "scheduled" {
		t.Errorf("state = %q, want %q -- a job scheduled an hour out must not be immediately available", state, "scheduled")
	}
	if diff := scheduledAt.Sub(runAt); diff < -time.Second || diff > time.Second {
		t.Errorf("scheduled_at = %v, want approximately %v (diff %v)", scheduledAt, runAt, diff)
	}
}
