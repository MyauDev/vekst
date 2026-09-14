package dedup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/ledger"
)

// TransferWindowDays is design D5's own pairing window: opposite signs,
// equal absolute amount, different accounts, within ±3 days.
const TransferWindowDays = 3

// Transfer is one detected pair, live until dismissed.
type Transfer struct {
	ID          uuid.UUID
	OutTxnID    uuid.UUID
	InTxnID     uuid.UUID
	DetectedAt  time.Time
	DismissedBy uuid.UUID
	DismissedAt time.Time
}

// baseAmount is the signed amount a pair is matched on -- the
// organisation's base currency, always, per migration 007's all-or-nothing
// FX guarantee: a transaction with no conversion is already in that
// currency, so its own amount_minor already is the base amount (see
// dedup.sql's own comment on TransferCandidatesForTransaction).
func baseAmount(t ledger.Transaction) int64 {
	if t.FX != nil {
		return t.FX.Base.MinorUnits
	}
	return t.Amount.MinorUnits
}

// DetectAndPair looks for one pairing candidate for txn -- already
// inserted, with a real id -- among the same entity's other transactions,
// and pairs the nearest one deterministically (design D5) if it finds one.
// nil, nil means txn is not one side of a transfer, which is the ordinary
// answer for almost every transaction.
//
// Excluded from the P&L on detection, not on confirmation (design D4,
// confirmed with the founder): pairing alone is what a later change's
// report query reads to exclude both sides -- see PairedTransactionIDs.
func DetectAndPair(ctx context.Context, tx pgx.Tx, org db.OrgID, entityID uuid.UUID, txn ledger.Transaction) (*Transfer, error) {
	amount := baseAmount(txn)
	if amount == 0 {
		// A zero-amount row has no sign to match against, and pairing two
		// of them would be a coincidence of nothing.
		return nil, nil
	}

	rows, err := gendb.New(tx).TransferCandidatesForTransaction(ctx, gendb.TransferCandidatesForTransactionParams{
		EntityID:   pgtype.UUID{Bytes: entityID, Valid: true},
		Column2:    int32(TransferWindowDays),
		ID:         pgtype.UUID{Bytes: txn.ID, Valid: true},
		Column4:    amount,
		AccountID:  pgtype.UUID{Bytes: txn.AccountID, Valid: true},
		Column6:    pgtype.Date{Time: txn.BookedOn, Valid: true},
		SourceKind: txn.SourceKind,
	})
	if err != nil {
		return nil, fmt.Errorf("dedup: finding transfer candidates: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	// Already ordered nearest by date then lowest id (design D5); the
	// query's own ORDER BY is what makes greedy correct here.
	candidateID := uuid.UUID(rows[0].ID.Bytes)

	outID, inID := txn.ID, candidateID
	if amount > 0 {
		// txn is the income side; the candidate, with the opposite sign, is
		// the money leaving the other account.
		outID, inID = candidateID, txn.ID
	}

	row, err := gendb.New(tx).InsertInternalTransferPair(ctx, gendb.InsertInternalTransferPairParams{
		OrgID:   pgtype.UUID{Bytes: org.UUID(), Valid: true},
		Column2: pgtype.UUID{Bytes: outID, Valid: true},
		Column3: pgtype.UUID{Bytes: inID, Valid: true},
	})
	if err != nil {
		// internal_transfer_members' own primary key refused this --
		// candidate was paired by something else between the read above and
		// this write. Not this transaction's problem to solve twice: the
		// next persist (if any) will see it already taken and skip it via
		// the same NOT EXISTS the candidates query already applies.
		return nil, fmt.Errorf("dedup: pairing transfer: %w", err)
	}

	return &Transfer{
		ID:         uuid.UUID(row.ID.Bytes),
		OutTxnID:   uuid.UUID(row.OutTxnID.Bytes),
		InTxnID:    uuid.UUID(row.InTxnID.Bytes),
		DetectedAt: row.DetectedAt.Time,
	}, nil
}

// ListActiveTransfers returns every undismissed pair touching entityID, on
// either side.
func ListActiveTransfers(ctx context.Context, tx pgx.Tx, entityID uuid.UUID) ([]Transfer, error) {
	rows, err := gendb.New(tx).ListActiveInternalTransfers(ctx, pgtype.UUID{Bytes: entityID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("dedup: listing internal transfers: %w", err)
	}
	transfers := make([]Transfer, len(rows))
	for i, row := range rows {
		transfers[i] = transferFromRow(row)
	}
	return transfers, nil
}

// ErrTransferNotFound means no active pair exists for that id -- already
// dismissed, never existed, or row-level security filtered it. The same
// non-disclosure every lookup in this schema gives: a policy denial, not a
// distinguishable not-found.
var ErrTransferNotFound = errors.New("dedup: no active internal transfer for that id")

// Dismiss returns a pair to the P&L and returns it as it now stands.
func Dismiss(ctx context.Context, tx pgx.Tx, transferID, userID uuid.UUID) (Transfer, error) {
	row, err := gendb.New(tx).DismissInternalTransfer(ctx, gendb.DismissInternalTransferParams{
		ID:          pgtype.UUID{Bytes: transferID, Valid: true},
		DismissedBy: pgtype.UUID{Bytes: userID, Valid: true},
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Transfer{}, ErrTransferNotFound
	case err != nil:
		return Transfer{}, fmt.Errorf("dedup: dismissing transfer: %w", err)
	}
	return transferFromRow(gendb.InternalTransfer(row)), nil
}

// PairedTransactionIDs is task 4.5: the query a later change (the P&L)
// uses to exclude paired transactions from every line. Unused by this
// change itself -- add-dedup excludes nothing from a report that does not
// exist yet; it only detects and records pairs.
func PairedTransactionIDs(ctx context.Context, tx pgx.Tx, entityID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := gendb.New(tx).PairedTransactionIDsForEntity(ctx, pgtype.UUID{Bytes: entityID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("dedup: listing paired transaction ids: %w", err)
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = uuid.UUID(row.Bytes)
	}
	return ids, nil
}

func transferFromRow(row gendb.InternalTransfer) Transfer {
	t := Transfer{
		ID:         uuid.UUID(row.ID.Bytes),
		OutTxnID:   uuid.UUID(row.OutTxnID.Bytes),
		InTxnID:    uuid.UUID(row.InTxnID.Bytes),
		DetectedAt: row.DetectedAt.Time,
	}
	if row.DismissedBy.Valid {
		t.DismissedBy = uuid.UUID(row.DismissedBy.Bytes)
	}
	if row.DismissedAt.Valid {
		t.DismissedAt = row.DismissedAt.Time
	}
	return t
}
