package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

// Cursor is a position in a batch's unclassified rows.
//
// A position and not an offset, and the difference is not a preference. The
// query used to rely on rows leaving the result as they were classified, so the
// next call returned the next page -- which holds only if every row gets
// classified, and the entire purpose of a confidence threshold is that some do
// not. One below-threshold row in a batch would hand the same page back
// forever.
type Cursor struct {
	BookedOn time.Time
	ID       uuid.UUID
}

// UnclassifiedTransactions reads one page of a batch's rows that no live
// classification answers, oldest first.
//
// Scoped to a batch because the job that reads it was enqueued for one: two
// imports running at once would otherwise classify each other's rows and each
// count them as its own.
//
// "No live classification" means neither superseded nor retracted. Those are
// two different events -- a correction names the classification that replaced
// it, an undone review decision has no successor to point at -- and a row whose
// only answer was retracted is a row nobody has answered.
func UnclassifiedTransactions(
	ctx context.Context,
	tx pgx.Tx,
	batchID uuid.UUID,
	cursor Cursor,
	limit int32,
) ([]Transaction, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("ledger: a page of %d transactions is not a page", limit)
	}

	rows, err := gendb.New(tx).UnclassifiedTransactions(ctx, gendb.UnclassifiedTransactionsParams{
		BatchID:        pgtype.UUID{Bytes: batchID, Valid: true},
		CursorBookedOn: pgtype.Date{Time: cursor.BookedOn, Valid: cursor.ID != uuid.Nil},
		CursorID:       pgtype.UUID{Bytes: cursor.ID, Valid: cursor.ID != uuid.Nil},
		RowLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("ledger: reading unclassified transactions: %w", err)
	}

	out := make([]Transaction, len(rows))
	for i, row := range rows {
		out[i], err = transactionFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("ledger: reading transaction %d: %w", i, err)
		}
	}
	return out, nil
}
