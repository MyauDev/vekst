package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/blob"
	"github.com/MyauDev/vekst/core/internal/db"
)

// ExpireBatchArgs runs once, scheduled by CreateImportBatch for
// upload_expires_at (design: a per-batch job, not a scheduled sweep -- a
// sweep would have no tenant context of its own to run under).
type ExpireBatchArgs struct {
	db.TenantJobArgs
	BatchID uuid.UUID
}

// Kind satisfies river.JobArgs.
func (ExpireBatchArgs) Kind() string { return "ingest_expire_batch" }

type expiryWorker struct {
	river.WorkerDefaults[ExpireBatchArgs]
	database *db.DB
	store    blob.ObjectStore
}

// Work abandons the batch if and only if the upload never arrived. Task 6.6:
// a batch already past awaiting_upload -- a real upload, or any later
// outcome -- is untouched, including its object. AbandonExpiredBatch's own
// WHERE clause is what makes this safe under a race: exactly one UPDATE can
// match "still awaiting_upload", so a confirm that lands first always wins.
func (w *expiryWorker) Work(ctx context.Context, job *river.Job[ExpireBatchArgs]) error {
	org, err := db.OrgIDFromJobArgs(job.Args.TenantJobArgs)
	if err != nil {
		return fmt.Errorf("ingest: expire_batch: %w", err)
	}
	batchID := pgtype.UUID{Bytes: job.Args.BatchID, Valid: true}

	abandoned := false
	err = w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).AbandonExpiredBatch(ctx, batchID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// The upload already arrived, or the batch already moved on some
			// other way. Not an error -- this is the expected shape of "lost
			// the race to a real upload."
			return nil
		case err != nil:
			return err
		default:
			abandoned = true
			return nil
		}
	})
	if err != nil {
		return fmt.Errorf("ingest: expire_batch: %w", err)
	}
	if !abandoned {
		return nil
	}

	// Delete is safe to call whether or not an object was ever written:
	// ObjectStore.Delete treats a missing key as success, which is exactly
	// "if one exists" without this worker needing to ask first.
	if err := w.store.Delete(ctx, blob.Key(org.UUID(), job.Args.BatchID)); err != nil {
		return fmt.Errorf("ingest: expire_batch: deleting object: %w", err)
	}
	return nil
}
