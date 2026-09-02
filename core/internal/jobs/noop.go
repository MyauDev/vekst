package jobs

import (
	"context"

	"github.com/riverqueue/river"
)

// NoopArgs is a job that proves the wiring -- River, the transactional
// enqueue, the start/stop lifecycle -- end to end. It does nothing. A real
// job arrives with ingest (2.2) and classification (3.2).
type NoopArgs struct{}

// Kind satisfies river.JobArgs.
func (NoopArgs) Kind() string { return "noop" }

// NoopWorker executes NoopArgs. It reads no job arguments and sets no tenant
// context, because it has none to set -- see the package doc for why every
// future worker must.
type NoopWorker struct {
	river.WorkerDefaults[NoopArgs]
}

// Work does nothing. Its only job is to have run.
func (w *NoopWorker) Work(_ context.Context, _ *river.Job[NoopArgs]) error {
	return nil
}
