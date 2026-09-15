package ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
)

// PersistRawRows writes one raw_rows row per parsed line, so a parse can be
// re-examined later without re-parsing the original file through whatever
// version of the parser is deployed at the time (task 4.1, ARCHITECTURE.md
// §5.5).
//
// The payload is the parsed cells (Row, JSON-marshaled) rather than the
// decoded text (task 4.3): decoding is a pure function of the object store's
// original bytes, so the decoded text can always be reproduced exactly and
// storing it again would be redundant. What is not reproducible after the
// fact is what *this* parse decided -- a later bug fix in cell extraction
// changes what a fresh parse would say, and payload_jsonb is the snapshot of
// what it actually said when this batch was persisted.
//
// It takes tx rather than opening its own transaction: this is one step of a
// larger unit of work -- persisting the rows and moving the batch to
// `parsed` belong in the same commit -- and the caller already holds the
// tenant context db.InTx set.
func PersistRawRows(ctx context.Context, tx pgx.Tx, org db.OrgID, batchID uuid.UUID, rows []Row) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	lineNos := make([]int32, len(rows))
	payloads := make([]string, len(rows))
	for i, r := range rows {
		payload, err := json.Marshal(r)
		if err != nil {
			return 0, fmt.Errorf("ingest: marshaling line %d: %w", r.LineNo, err)
		}
		lineNos[i] = int32(r.LineNo)
		payloads[i] = string(payload)
	}

	n, err := gendb.New(tx).InsertRawRows(ctx, gendb.InsertRawRowsParams{
		OrgID:   pgtype.UUID{Bytes: org.UUID(), Valid: true},
		BatchID: pgtype.UUID{Bytes: batchID, Valid: true},
		Column3: lineNos,
		Column4: payloads,
	})
	if err != nil {
		return 0, fmt.Errorf("ingest: persisting raw rows: %w", err)
	}
	return n, nil
}
