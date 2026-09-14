// Package dedup is D1-D5 (change 2.6): the same file twice, the same row
// twice within a batch or against everything the organisation has already
// imported, and internal-transfer pairing. It holds the decision logic and
// the tables that record what it decided; core/internal/ingest's persist
// job is the caller that owns when in the pipeline this runs.
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
)

// Level values dedup_skips admits. LevelD2 is never written by this
// package's own code -- see partition.go's own doc comment for why an
// in-batch collision is unreachable once dedup_hash carries occurrence --
// but the schema still admits it, and GetDedupSummary still counts it, on
// the chance a future parser's own bug produces one.
const (
	LevelD2 = "D2"
	LevelD3 = "D3"
)

// Skip is one row a customer's file contained that a report will not: the
// line it came from, why, and -- for D3 -- what it matched. Recorded rather
// than merely counted (design D3), because the hash has ordinary false
// positives and a customer who disagrees has to be able to see exactly
// which line and against what.
type Skip struct {
	ID        uuid.UUID
	BatchID   uuid.UUID
	LineNo    int
	PostingNo int32
	// D2 or D3 -- see this file's own note on LevelD2.
	Level     string
	DedupHash string
	// Set for D3 only; the zero UUID for D2, where the original would be
	// another line of this same file rather than an existing transaction.
	MatchedTransactionID uuid.UUID
	MatchedBatchID       uuid.UUID
	CreatedAt            time.Time
}

// InsertSkip records one skip. Written once -- migration 012 revokes
// vekst_app's UPDATE on dedup_skips entirely.
func InsertSkip(ctx context.Context, tx pgx.Tx, org db.OrgID, s Skip) (Skip, error) {
	row, err := gendb.New(tx).InsertDedupSkip(ctx, gendb.InsertDedupSkipParams{
		OrgID:                pgtype.UUID{Bytes: org.UUID(), Valid: true},
		BatchID:              pgtype.UUID{Bytes: s.BatchID, Valid: true},
		LineNo:               int32(s.LineNo),
		PostingNo:            int16(s.PostingNo),
		Level:                s.Level,
		DedupHash:            s.DedupHash,
		MatchedTransactionID: pgtype.UUID{Bytes: s.MatchedTransactionID, Valid: s.MatchedTransactionID != uuid.Nil},
		MatchedBatchID:       pgtype.UUID{Bytes: s.MatchedBatchID, Valid: s.MatchedBatchID != uuid.Nil},
	})
	if err != nil {
		return Skip{}, fmt.Errorf("dedup: recording skip: %w", err)
	}
	return skipFromRow(row), nil
}

// ListSkips returns every skip recorded for a batch, in file order.
func ListSkips(ctx context.Context, tx pgx.Tx, batchID uuid.UUID) ([]Skip, error) {
	rows, err := gendb.New(tx).ListDedupSkipsForBatch(ctx, pgtype.UUID{Bytes: batchID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("dedup: listing skips: %w", err)
	}
	skips := make([]Skip, len(rows))
	for i, row := range rows {
		skips[i] = skipFromRow(row)
	}
	return skips, nil
}

// CountSkips is GetDedupSummary's own read: derived from the table, never
// from a stored counter (design D3, task 3.5).
func CountSkips(ctx context.Context, tx pgx.Tx, batchID uuid.UUID, level string) (int32, error) {
	n, err := gendb.New(tx).CountDedupSkipsForBatch(ctx, gendb.CountDedupSkipsForBatchParams{
		BatchID: pgtype.UUID{Bytes: batchID, Valid: true},
		Level:   level,
	})
	if err != nil {
		return 0, fmt.Errorf("dedup: counting %s skips: %w", level, err)
	}
	return int32(n), nil
}

// FindByHash is D3's cross-batch lookup: does any transaction, in any of
// this organisation's batches, already carry this exact hash. found is
// false, not an error, when nothing matches -- that is the expected answer
// for almost every row.
func FindByHash(ctx context.Context, tx pgx.Tx, hash string) (transactionID, batchID uuid.UUID, found bool, err error) {
	row, err := gendb.New(tx).FindTransactionByDedupHash(ctx, hash)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return uuid.Nil, uuid.Nil, false, nil
	case err != nil:
		return uuid.Nil, uuid.Nil, false, fmt.Errorf("dedup: finding by hash: %w", err)
	}
	return uuid.UUID(row.ID.Bytes), uuid.UUID(row.BatchID.Bytes), true, nil
}

func skipFromRow(row gendb.DedupSkip) Skip {
	s := Skip{
		ID:        uuid.UUID(row.ID.Bytes),
		BatchID:   uuid.UUID(row.BatchID.Bytes),
		LineNo:    int(row.LineNo),
		PostingNo: int32(row.PostingNo),
		Level:     row.Level,
		DedupHash: row.DedupHash,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.MatchedTransactionID.Valid {
		s.MatchedTransactionID = uuid.UUID(row.MatchedTransactionID.Bytes)
	}
	if row.MatchedBatchID.Valid {
		s.MatchedBatchID = uuid.UUID(row.MatchedBatchID.Bytes)
	}
	return s
}
