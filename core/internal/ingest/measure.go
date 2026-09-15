package ingest

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/blob"
	"github.com/MyauDev/vekst/core/internal/db"
)

// MeasureUploadArgs enqueues the job design D2 calls "core reads its own
// mail": ConfirmImportUpload records nothing itself, and this is the job it
// hands off to. Arguments are identifiers, never file content (CLAUDE.md: job
// arguments are not tenant-isolated).
type MeasureUploadArgs struct {
	db.TenantJobArgs
	BatchID uuid.UUID
}

// Kind satisfies river.JobArgs.
func (MeasureUploadArgs) Kind() string { return "ingest_measure_upload" }

// failureUploadMissing, failureFileTooLarge and failureAlreadyImported are
// the failure_code values this job can write. All three are codes, never
// sentences (CLAUDE.md).
const (
	failureUploadMissing   = "upload_missing"
	failureFileTooLarge    = "file_too_large"
	failureAlreadyImported = "already_imported"
)

type measurementWorker struct {
	river.WorkerDefaults[MeasureUploadArgs]
	database *db.DB
	store    blob.ObjectStore
	cfg      Config
}

// Work is design D2 end to end: HEAD, then a bounded stream through SHA-256
// and a byte counter, then one write. It never trusts a number the browser
// supplied -- not the declared size, not the declared type -- because core
// never held the bytes when those numbers were written.
func (w *measurementWorker) Work(ctx context.Context, job *river.Job[MeasureUploadArgs]) error {
	org, err := db.OrgIDFromJobArgs(job.Args.TenantJobArgs)
	if err != nil {
		return fmt.Errorf("ingest: measure_upload: %w", err)
	}
	batchID := pgtype.UUID{Bytes: job.Args.BatchID, Valid: true}

	var batch gendb.GetImportBatchRow
	err = w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		batch, err = gendb.New(tx).GetImportBatch(ctx, batchID)
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// The batch is gone or never belonged to this organisation -- RLS
		// filtered it. Nothing for this job to do; retrying would not change
		// the answer.
		return nil
	case err != nil:
		return fmt.Errorf("ingest: measure_upload: reading batch: %w", err)
	}

	// Idempotent re-run (task 6.10): a batch already past this job's concern
	// -- abandoned, failed, or moved on to a later stage -- is left alone.
	// awaiting_upload and uploaded are both accepted so that running this job
	// twice against a successful upload re-measures and re-writes the same
	// values, which is safe because reading an immutable object twice gives
	// the same answer.
	if batch.Status != string(StatusAwaitingUpload) && batch.Status != string(StatusUploaded) {
		return nil
	}

	key := blob.Key(org.UUID(), job.Args.BatchID)

	if err := w.store.Head(ctx, key); err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return w.fail(ctx, org, batchID, failureUploadMissing)
		}
		return fmt.Errorf("ingest: measure_upload: head: %w", err)
	}

	rc, err := w.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			// Raced with something that deleted the object between Head and
			// Get -- treated the same as it never having arrived.
			return w.fail(ctx, org, batchID, failureUploadMissing)
		}
		return fmt.Errorf("ingest: measure_upload: get: %w", err)
	}
	defer rc.Close()

	head := &headCapture{max: sniffWindow}
	hasher := sha256.New()
	// LimitReader is the hard stop: it never delivers more than
	// UploadMaxBytes+1 bytes to Copy however large the object actually is,
	// which is what keeps a hostile upload from being read into memory (or
	// even off the wire) without bound.
	n, copyErr := io.Copy(io.MultiWriter(hasher, head), io.LimitReader(rc, w.cfg.UploadMaxBytes+1))
	if copyErr != nil {
		return fmt.Errorf("ingest: measure_upload: reading object: %w", copyErr)
	}

	if n > w.cfg.UploadMaxBytes {
		if err := w.store.Delete(ctx, key); err != nil {
			return fmt.Errorf("ingest: measure_upload: deleting oversized object: %w", err)
		}
		return w.fail(ctx, org, batchID, failureFileTooLarge)
	}

	contentType := sniffContentType(head.buf)
	fileSHA256 := hasher.Sum(nil)

	// D1 (add-dedup, change 2.6, design D1). Looks first, so the ordinary
	// case -- a customer re-uploading a file they already imported -- gets
	// a named failure instead of a raw constraint violation surfacing
	// later. The guarantee that two concurrent uploads of the same file
	// cannot both reach imported does not depend on this check: it is
	// import_batches_file_once (migration 012), enforced where a batch's
	// status actually becomes 'imported' -- core/internal/ingest's persist
	// job -- which is the backstop this early check narrows the race
	// window for but does not replace.
	var alreadyImported bool
	err = w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).FindImportedBatchAlreadyHoldingThisFile(ctx, fileSHA256)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil
		case err != nil:
			return err
		}
		alreadyImported = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("ingest: measure_upload: checking for a prior import: %w", err)
	}
	if alreadyImported {
		return w.fail(ctx, org, batchID, failureAlreadyImported)
	}

	err = w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		affected, err := gendb.New(tx).RecordUploadMeasurement(ctx, gendb.RecordUploadMeasurementParams{
			ID:          batchID,
			FileSha256:  fileSHA256,
			ByteLength:  pgtype.Int8{Int64: n, Valid: true},
			ContentType: pgtype.Text{String: contentType, Valid: true},
		})
		if err := db.ExactlyOneRow(affected, err); err != nil {
			return err
		}

		// Chained in the same transaction the measurement commits in, via
		// river.ClientFromContext rather than a *jobs.Client field: Workers
		// deliberately holds none (see its own doc comment), and River
		// makes the client that is already running this job available
		// through its own Work() context for exactly this case.
		_, err = river.ClientFromContext[pgx.Tx](ctx).InsertTx(ctx, tx, ValidateImportArgs{
			TenantJobArgs: db.TenantJobArgs{OrgID: org.UUID()},
			BatchID:       job.Args.BatchID,
		}, nil)
		return err
	})
	if err != nil {
		return fmt.Errorf("ingest: measure_upload: recording measurement: %w", err)
	}
	return nil
}

// fail writes a terminal failure. It is used for the two ways this job can
// discover a batch cannot be measured -- design D2 -- and nothing else calls
// SetImportBatchStatus with a failure_code from this file.
func (w *measurementWorker) fail(ctx context.Context, org db.OrgID, batchID pgtype.UUID, code string) error {
	err := w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		affected, err := gendb.New(tx).SetImportBatchStatus(ctx, gendb.SetImportBatchStatusParams{
			ID:          batchID,
			Status:      string(StatusFailed),
			FailureCode: pgtype.Text{String: code, Valid: true},
		})
		return db.ExactlyOneRow(affected, err)
	})
	if err != nil {
		return fmt.Errorf("ingest: measure_upload: recording %s: %w", code, err)
	}
	return nil
}

// headCapture retains only the first max bytes written to it and discards the
// rest, while still reporting every byte as consumed. It lets io.MultiWriter
// feed sniffContentType the same stream sha256 sees without holding the whole
// object in memory to get eight kilobytes of it.
type headCapture struct {
	buf []byte
	max int
}

func (h *headCapture) Write(p []byte) (int, error) {
	if len(h.buf) < h.max {
		need := h.max - len(h.buf)
		if need > len(p) {
			need = len(p)
		}
		h.buf = append(h.buf, p[:need]...)
	}
	return len(p), nil
}
