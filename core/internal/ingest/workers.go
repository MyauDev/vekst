package ingest

import (
	"time"

	"github.com/riverqueue/river"

	"github.com/MyauDev/vekst/core/internal/blob"
	"github.com/MyauDev/vekst/core/internal/db"
)

// Config is what both the workers and the Service need from
// core/internal/config.Config: the two fields add-file-upload design names
// UploadMaxBytes and UploadURLLifetime.
type Config struct {
	UploadMaxBytes    int64
	UploadURLLifetime time.Duration
}

// Workers registers this package's River workers with core/internal/jobs.
//
// It deliberately holds no *jobs.Client. The measurement worker does chain
// into the next stage -- add-ingest-validation's ValidateImportArgs -- but
// it reaches the client already running it through river.ClientFromContext
// inside its own Work(), not through a field here, so Workers can still be
// built and handed to jobs.New before a *jobs.Client exists at all. Service,
// below, is the other side of that same cycle -- it enqueues jobs from
// CreateImportBatch and ConfirmImportUpload, so it needs the *jobs.Client
// jobs.New returns, and is built after it rather than passed into it.
type Workers struct {
	database *db.DB
	store    blob.ObjectStore
	cfg      Config
}

// NewWorkers builds the registrar. It performs no I/O.
func NewWorkers(database *db.DB, store blob.ObjectStore, cfg Config) *Workers {
	return &Workers{database: database, store: store, cfg: cfg}
}

// Register satisfies core/internal/jobs.WorkerRegistrar.
func (w *Workers) Register(rw *river.Workers) []*river.PeriodicJob {
	river.AddWorker(rw, &measurementWorker{database: w.database, store: w.store, cfg: w.cfg})
	river.AddWorker(rw, &expiryWorker{database: w.database, store: w.store})
	river.AddWorker(rw, &validateWorker{database: w.database, store: w.store, now: time.Now})
	river.AddWorker(rw, &persistWorker{database: w.database, store: w.store})
	// No periodic schedule: every job here is enqueued per-batch -- from
	// Service, or chained from another job in the same transaction -- never
	// swept. Design's rejected-alternatives table: a scan has no tenant
	// context of its own to run under.
	return nil
}
