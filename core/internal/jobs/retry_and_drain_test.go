package jobs

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

// immediateRetryPolicy makes a failed job eligible for retry right away,
// instead of River's default exponential backoff (seconds to minutes). These
// two tests are about proving River's own retry/drain guarantees hold in
// this wiring, not about testing how fast River backs off.
type immediateRetryPolicy struct{}

func (immediateRetryPolicy) NextRetry(*rivertype.JobRow) time.Time {
	return time.Now()
}

type flakyArgs struct{}

func (flakyArgs) Kind() string { return "flaky" }

// flakyWorker fails its first attempt and succeeds every attempt after.
type flakyWorker struct {
	river.WorkerDefaults[flakyArgs]
	attempts atomic.Int32
}

func (w *flakyWorker) Work(_ context.Context, _ *river.Job[flakyArgs]) error {
	if w.attempts.Add(1) == 1 {
		return errors.New("flaky: failing on purpose, first attempt only")
	}
	return nil
}

// Task/spec scenario: "A failing job is retried rather than lost." Built as
// a standalone client rather than through jobs.New, since this test needs a
// non-default retry policy and jobs.go's production wiring has no reason to
// expose one.
func TestFailedJobIsRetried(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worker := &flakyWorker{}
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:      map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers:     workers,
		RetryPolicy: immediateRetryPolicy{},
	})
	if err != nil {
		t.Fatalf("river.NewClient: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Stop(stopCtx)
	}()

	if _, err := client.Insert(ctx, flakyArgs{}, nil); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if worker.attempts.Load() >= 2 {
			return // ran, failed, retried, and ran again -- exactly what the scenario asserts
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("job was attempted %d time(s) in 10s, want at least 2 (fail then retry)", worker.attempts.Load())
}

type slowArgs struct{}

func (slowArgs) Kind() string { return "slow" }

type slowWorker struct {
	river.WorkerDefaults[slowArgs]
}

func (w *slowWorker) Work(ctx context.Context, _ *river.Job[slowArgs]) error {
	select {
	case <-time.After(500 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Task/spec scenario: "Shutdown drains rather than abandons." A job that is
// still running when Stop is called must finish -- not be left half-run or
// silently cancelled -- as long as it finishes within SoftStopTimeout.
func TestStopDrainsRunningJob(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set; skipping a test that needs a live, migrated Postgres")
	}
	pool := testPool(t)
	ctx := context.Background()

	workers := river.NewWorkers()
	river.AddWorker(workers, &slowWorker{})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:          map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers:         workers,
		SoftStopTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("river.NewClient: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, err := client.Insert(ctx, slowArgs{}, nil); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Wait for the job to actually be running before stopping -- the point
	// is to prove a *running* job gets drained, not merely that Stop doesn't
	// crash on an empty queue. A fixed sleep was flaky: River's dispatch
	// isn't instant, and a stop that lands before the job starts leaves it
	// 'available', not 'running', which wouldn't test drain at all.
	runningDeadline := time.Now().Add(3 * time.Second)
	var state string
	for time.Now().Before(runningDeadline) {
		if err := pool.QueryRow(ctx, "SELECT state FROM river_job WHERE kind = 'slow' ORDER BY id DESC LIMIT 1").Scan(&state); err != nil {
			t.Fatalf("querying river_job: %v", err)
		}
		if state == "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if state != "running" {
		t.Fatalf("job never reached 'running' before the drain deadline; last state %q", state)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v (a running job should be drained, not fail shutdown)", err)
	}

	if err := pool.QueryRow(ctx, "SELECT state FROM river_job WHERE kind = 'slow' ORDER BY id DESC LIMIT 1").Scan(&state); err != nil {
		t.Fatalf("querying river_job: %v", err)
	}
	if state != "completed" {
		t.Fatalf("job state after a drained Stop = %q, want %q -- it was abandoned mid-run, not drained", state, "completed")
	}
}
