package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/blob"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/dedup"
	"github.com/MyauDev/vekst/core/internal/ledger"
	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/normalize"
)

// failurePersistReparseFailed is a defect in us: this batch already parsed
// successfully once, during validation, from the same object under the
// same profile. Reaching here and failing to parse again means something
// changed underneath the pipeline -- the object, the profile -- not that
// the customer's file was ever bad.
const failurePersistReparseFailed = "persist_reparse_failed"

// PersistImportArgs runs change 2.6's D1-D5 (D1 is measure.go's own job;
// this is D2-D5) and, if nothing is skipped or paired away, moves the
// batch to imported. Enqueued by the validate job (add-ingest-validation)
// the moment a batch reaches validated -- warnings or not; an override
// changes what a report shows, never whether a valid batch persists.
type PersistImportArgs struct {
	db.TenantJobArgs
	BatchID uuid.UUID
}

// Kind satisfies river.JobArgs.
func (PersistImportArgs) Kind() string { return "ingest_persist_import" }

type persistWorker struct {
	river.WorkerDefaults[PersistImportArgs]
	database *db.DB
	store    blob.ObjectStore
}

// Work re-parses the object -- raw_rows is an audit trail of what was
// parsed, not an input to reparse from (PersistRawRows' own doc comment),
// and Statement-level facts (currency, the account identifier) live on the
// parse, not on any one row -- resolves the account, deduplicates, inserts
// what survives, pairs internal transfers among the newly inserted rows,
// and lands the batch on imported. Everything after account resolution is
// one db.InTx: a batch is never half-deduplicated (task 3.4).
func (w *persistWorker) Work(ctx context.Context, job *river.Job[PersistImportArgs]) error {
	org, err := db.OrgIDFromJobArgs(job.Args.TenantJobArgs)
	if err != nil {
		return fmt.Errorf("ingest: persist_import: %w", err)
	}
	batchID := pgtype.UUID{Bytes: job.Args.BatchID, Valid: true}

	batch, params, err := loadBatchAndProfile(ctx, w.database, org, batchID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil // gone, or never belonged to this organisation; RLS filtered it.
	case err != nil:
		return fmt.Errorf("ingest: persist_import: reading batch: %w", err)
	}

	// Idempotent re-run, the same shape as measure and validate: a batch
	// already past this job's concern is left alone.
	if batch.Status != string(StatusValidated) {
		return nil
	}

	raw, err := readObject(ctx, w.store, blob.Key(org.UUID(), job.Args.BatchID))
	if err != nil {
		return fmt.Errorf("ingest: persist_import: reading object: %w", err)
	}

	st, parseErr := ParseWithParams(raw, params)
	if parseErr != nil {
		return w.fail(ctx, org, batchID, failurePersistReparseFailed)
	}

	entityID := uuid.UUID(batch.EntityID.Bytes)

	err = w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		accountID, _, err := ResolveAccount(ctx, tx, org, entityID, st)
		if err != nil {
			return fmt.Errorf("resolving account: %w", err)
		}

		candidates := make([]dedup.Candidate, len(st.Rows))
		for i, r := range st.Rows {
			txn, err := buildTransaction(entityID, accountID, job.Args.BatchID, batch.SourceKind, st.Currency, r)
			if err != nil {
				return fmt.Errorf("line %d: %w", r.LineNo, err)
			}
			candidates[i] = dedup.Candidate{LineNo: r.LineNo, Txn: txn}
		}

		// D2: assigns each row its occurrence-aware hash. Two genuinely
		// repeated rows in the file get different occurrences and are both
		// kept -- see dedup.Partition's own doc comment for why there is
		// no skip logic left to run here.
		withHashes := dedup.Partition(candidates)

		// D3: against everything this organisation has already imported.
		var toInsert []ledger.Transaction
		var skips []dedup.Skip
		for _, c := range withHashes {
			matchedTxnID, matchedBatchID, found, err := dedup.FindByHash(ctx, tx, c.Txn.DedupHash)
			if err != nil {
				return fmt.Errorf("checking line %d against prior imports: %w", c.LineNo, err)
			}
			if found {
				skips = append(skips, dedup.Skip{
					LineNo: c.LineNo, PostingNo: c.Txn.PostingNo,
					Level: dedup.LevelD3, DedupHash: c.Txn.DedupHash,
					MatchedTransactionID: matchedTxnID, MatchedBatchID: matchedBatchID,
				})
				continue
			}
			toInsert = append(toInsert, c.Txn)
		}

		for _, s := range skips {
			s.BatchID = job.Args.BatchID
			if _, err := dedup.InsertSkip(ctx, tx, org, s); err != nil {
				return fmt.Errorf("recording skip at line %d: %w", s.LineNo, err)
			}
		}

		inserted, err := ledger.Insert(ctx, tx, org, toInsert)
		if err != nil {
			return fmt.Errorf("inserting transactions: %w", err)
		}

		// D4/D5: pair internal transfers among the rows this batch just
		// added. Excluded from the P&L on detection (design D4, confirmed
		// with the founder); pairing itself is all this does.
		for _, t := range inserted {
			if _, err := dedup.DetectAndPair(ctx, tx, org, entityID, t); err != nil {
				return fmt.Errorf("pairing transfers for transaction %s: %w", t.ID, err)
			}
		}

		if err := CheckTransition(StatusValidated, StatusPersisting); err != nil {
			return err
		}
		if err := CheckTransition(StatusPersisting, StatusImported); err != nil {
			return err
		}

		affected, err := gendb.New(tx).SetImportBatchStatus(ctx, gendb.SetImportBatchStatusParams{
			ID:     batchID,
			Status: string(StatusImported),
		})
		return db.ExactlyOneRow(affected, err)
	})

	// D1's actual backstop: measure.go's own check narrows the race window
	// but does not close it -- two uploads of the same file can both reach
	// this point if they raced closely enough. import_batches_file_once
	// (migration 012) is what closes it, and a violation surfaces here,
	// against the same SetImportBatchStatus this InTx just ran, as the
	// InTx's own rollback of everything this job inserted -- nothing from
	// a losing race is left half-written.
	if isFileOnceViolation(err) {
		return w.fail(ctx, org, batchID, failureAlreadyImported)
	}
	return err
}

// isFileOnceViolation reports whether err is import_batches_file_once
// refusing this batch, by SQLSTATE and constraint name -- not by message,
// which the rest of this package's error checks avoid for the same reason.
func isFileOnceViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == "import_batches_file_once"
}

func (w *persistWorker) fail(ctx context.Context, org db.OrgID, batchID pgtype.UUID, code string) error {
	err := w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		affected, err := gendb.New(tx).SetImportBatchStatus(ctx, gendb.SetImportBatchStatusParams{
			ID:          batchID,
			Status:      string(StatusFailed),
			FailureCode: pgtype.Text{String: code, Valid: true},
		})
		return db.ExactlyOneRow(affected, err)
	})
	if err != nil {
		return fmt.Errorf("ingest: persist_import: recording %s: %w", code, err)
	}
	return nil
}

// buildTransaction turns one parsed Row into a ledger.Transaction. Every
// fixture and parser in this repository today is a bank export (no
// ledger-format parser exists yet), so this always produces the bank grain
// -- DocumentRef "", PostingNo 0 -- regardless of sourceKind; a ledger
// parser splitting one document into its postings is a later change's
// problem to solve where it is created, not one this function can invent
// an answer for today.
//
// amount_minor is signed: income positive, expense negative. direction is
// carried alongside it, denormalised for readability, the same reasoning
// source_kind is denormalised onto every row from its batch.
func buildTransaction(entityID, accountID, batchID uuid.UUID, sourceKind, currency string, r Row) (ledger.Transaction, error) {
	booked, err := ParseDate(r.BookedOn)
	if err != nil {
		return ledger.Transaction{}, fmt.Errorf("booked_on %q: %w", r.BookedOn, err)
	}

	direction := ledger.DirectionIncome
	amountMinor := r.Credit.MinorUnits
	if r.Debit.MinorUnits != 0 {
		direction = ledger.DirectionExpense
		amountMinor = -r.Debit.MinorUnits
	}

	counterpartyKey, _ := normalize.CounterpartyKey(r.CounterpartyName, r.CounterpartyTaxID, r.CounterpartyAccount)

	return ledger.Transaction{
		EntityID:         entityID,
		AccountID:        accountID,
		BatchID:          batchID,
		LineNo:           int32(r.LineNo),
		SourceKind:       sourceKind,
		Direction:        direction,
		BookedOn:         booked,
		Amount:           money.Money{CurrencyCode: currency, MinorUnits: amountMinor},
		CounterpartyRaw:  r.CounterpartyName,
		CounterpartyKey:  counterpartyKey,
		DescriptionRaw:   r.Description,
		DescriptionNorm:  normalize.Description(r.Description),
		NormalizeVersion: normalize.Version,
		RegulatedCode:    r.RegulatedCode,
		BankRef:          r.DocumentNo,
	}, nil
}
