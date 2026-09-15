package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/blob"
	"github.com/MyauDev/vekst/core/internal/db"
)

// failureUnrecognizedFormat is a defect in us -- no parser in the registry
// claims the file -- not a validation outcome, the same distinction design
// D3 (add-file-upload) draws between 'failed' and 'rejected'. It is not
// under a customer's control the way a malformed amount is: it means this
// product does not yet read their bank, not that their data is wrong.
const failureUnrecognizedFormat = "unrecognized_format"

// ValidateImportArgs runs the rest of the pipeline once a batch's upload has
// been measured: parse the object, persist its raw rows, validate, and land
// the batch on validated or rejected. Enqueued by the measurement job
// (add-file-upload) after a successful measurement -- there is nothing for a
// customer to choose between parsing and validating yet (import profiles,
// change 2.4, are a future change with nothing to insert here today), so one
// job does both rather than two jobs with nothing to do between them.
type ValidateImportArgs struct {
	db.TenantJobArgs
	BatchID uuid.UUID
}

// Kind satisfies river.JobArgs.
func (ValidateImportArgs) Kind() string { return "ingest_validate_import" }

type validateWorker struct {
	river.WorkerDefaults[ValidateImportArgs]
	database *db.DB
	store    blob.ObjectStore
	// now is time.Now except in tests, so the date-plausible check can be
	// pinned instead of racing the calendar (task 2.2).
	now func() time.Time
}

// Work reads the object, parses it, and validates it -- writing
// import_validations and moving the batch to its terminal state in one
// db.InTx (task 5.2), so a rejected batch's receipt and its status change
// together or not at all.
func (w *validateWorker) Work(ctx context.Context, job *river.Job[ValidateImportArgs]) error {
	org, err := db.OrgIDFromJobArgs(job.Args.TenantJobArgs)
	if err != nil {
		return fmt.Errorf("ingest: validate_import: %w", err)
	}
	batchID := pgtype.UUID{Bytes: job.Args.BatchID, Valid: true}

	batch, params, err := loadBatchAndProfile(ctx, w.database, org, batchID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil // gone, or never belonged to this organisation; RLS filtered it.
	case err != nil:
		return fmt.Errorf("ingest: validate_import: reading batch: %w", err)
	}

	// Idempotent re-run, the same shape as the measurement job: a batch
	// already past this job's concern -- validated, rejected, failed, or
	// abandoned -- is left alone.
	if batch.Status != string(StatusUploaded) {
		return nil
	}

	raw, err := readObject(ctx, w.store, blob.Key(org.UUID(), job.Args.BatchID))
	if err != nil {
		return fmt.Errorf("ingest: validate_import: reading object: %w", err)
	}

	st, parseErr := ParseWithParams(raw, params)
	if parseErr != nil {
		return w.fail(ctx, org, batchID, failureUnrecognizedFormat)
	}

	result := ValidateStatementWithParams(st, w.now(), params)

	return w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		if _, err := PersistRawRows(ctx, tx, org, job.Args.BatchID, st.Rows); err != nil {
			return err
		}

		if params != nil {
			resolvedJSON, err := params.resolvedParametersJSON()
			if err != nil {
				return fmt.Errorf("marshaling resolved parameters: %w", err)
			}
			if _, err := q.SetResolvedParameters(ctx, gendb.SetResolvedParametersParams{
				ID:                 batchID,
				ResolvedParameters: resolvedJSON,
			}); err != nil {
				return fmt.Errorf("storing resolved parameters: %w", err)
			}
		}

		_, mixedCurrency, resolveErr := ResolveAccount(ctx, tx, org, uuid.UUID(batch.EntityID.Bytes), st)
		result = MergeAccountResolution(result, resolveErr, mixedCurrency)

		reportJSON, err := BuildReportJSON(result)
		if err != nil {
			return fmt.Errorf("building report: %w", err)
		}

		var balancePassed pgtype.Bool
		if result.BalanceCheckPassed != nil {
			balancePassed = pgtype.Bool{Bool: *result.BalanceCheckPassed, Valid: true}
		}

		if _, err := q.InsertValidation(ctx, gendb.InsertValidationParams{
			OrgID:              pgtype.UUID{Bytes: org.UUID(), Valid: true},
			BatchID:            batchID,
			Outcome:            string(result.Outcome),
			RowCount:           int32(result.RowCount),
			ErrorCount:         int32(len(result.Errors)),
			WarningCount:       int32(len(result.Warnings)),
			BalanceCheckPassed: balancePassed,
			ReportJsonb:        reportJSON,
		}); err != nil {
			return fmt.Errorf("inserting validation: %w", err)
		}

		nextStatus := StatusValidated
		if result.Outcome == OutcomeRejected {
			nextStatus = StatusRejected
		}
		if err := CheckTransition(StatusUploaded, StatusParsing); err != nil {
			return err
		}
		if err := CheckTransition(StatusParsing, StatusParsed); err != nil {
			return err
		}
		if err := CheckTransition(StatusParsed, StatusValidating); err != nil {
			return err
		}
		if err := CheckTransition(StatusValidating, nextStatus); err != nil {
			return err
		}

		affected, err := q.SetImportBatchStatus(ctx, gendb.SetImportBatchStatusParams{
			ID:     batchID,
			Status: string(nextStatus),
		})
		if err := db.ExactlyOneRow(affected, err); err != nil {
			return err
		}

		if nextStatus != StatusValidated {
			return nil
		}
		// Chained the same way measure chains into validate (river.ClientFromContext,
		// not a held *jobs.Client): a valid file, warnings or not, has nothing
		// left for a person to do before its rows become transactions. An
		// override changes what the report shows about this batch, never
		// whether it persists (add-ingest-validation's own outcome states:
		// valid_with_warnings already reached 'validated', not a separate
		// gate this chain would need to wait behind).
		_, err = river.ClientFromContext[pgx.Tx](ctx).InsertTx(ctx, tx, PersistImportArgs{
			TenantJobArgs: db.TenantJobArgs{OrgID: org.UUID()},
			BatchID:       job.Args.BatchID,
		}, nil)
		return err
	})
}

func (w *validateWorker) fail(ctx context.Context, org db.OrgID, batchID pgtype.UUID, code string) error {
	err := w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		affected, err := gendb.New(tx).SetImportBatchStatus(ctx, gendb.SetImportBatchStatusParams{
			ID:          batchID,
			Status:      string(StatusFailed),
			FailureCode: pgtype.Text{String: code, Valid: true},
		})
		return db.ExactlyOneRow(affected, err)
	})
	if err != nil {
		return fmt.Errorf("ingest: validate_import: recording %s: %w", code, err)
	}
	return nil
}

// loadBatchAndProfile reads a batch and, if it names one, the import
// profile it was created with -- one InTx, shared by every job downstream
// of upload that needs to parse the object again (validate, and now
// persist). Returns the Parameters a parse should use: nil when the batch
// names no profile, which is what makes "no profile, no change" (task 6.1,
// add-import-profiles) true by construction rather than by a second code
// path.
func loadBatchAndProfile(ctx context.Context, database *db.DB, org db.OrgID, batchID pgtype.UUID) (gendb.GetImportBatchRow, *Parameters, error) {
	var batch gendb.GetImportBatchRow
	var profile *Profile
	err := database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)
		var err error
		batch, err = q.GetImportBatch(ctx, batchID)
		if err != nil {
			return err
		}
		if batch.ImportProfileID.Valid {
			row, err := q.GetImportProfile(ctx, batch.ImportProfileID)
			if err != nil {
				return fmt.Errorf("reading import profile: %w", err)
			}
			p, err := profileFromRow(row)
			if err != nil {
				return err
			}
			profile = &p
		}
		return nil
	})
	if err != nil {
		return gendb.GetImportBatchRow{}, nil, err
	}
	var params *Parameters
	if profile != nil {
		params = profile.toParameters()
	}
	return batch, params, nil
}

func readObject(ctx context.Context, store blob.ObjectStore, key string) ([]byte, error) {
	rc, err := store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
