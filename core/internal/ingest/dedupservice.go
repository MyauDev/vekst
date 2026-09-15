package ingest

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/dedup"
)

// Summary is the one number a batch's import screen shows -- "412 rows
// imported, 88 duplicates skipped" -- with D2, D3 and internal transfers
// broken out. Every count is derived from its own table (design D3, task
// 3.5), never from a counter this service maintains.
type Summary struct {
	ImportedRows      int32
	SkippedInBatch    int32
	SkippedCrossBatch int32
	InternalTransfers int32
}

// GetDedupSummary reads what one batch's persist decided.
func (s *Service) GetDedupSummary(ctx context.Context, userID, requestedOrgID, batchID uuid.UUID) (Summary, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return Summary{}, err
	}

	var summary Summary
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		batchPg := pgtype.UUID{Bytes: batchID, Valid: true}
		q := gendb.New(tx)

		imported, err := q.CountTransactionsForBatch(ctx, batchPg)
		if err != nil {
			return fmt.Errorf("counting transactions: %w", err)
		}
		summary.ImportedRows = int32(imported)

		d2, err := dedup.CountSkips(ctx, tx, batchID, dedup.LevelD2)
		if err != nil {
			return err
		}
		summary.SkippedInBatch = d2

		d3, err := dedup.CountSkips(ctx, tx, batchID, dedup.LevelD3)
		if err != nil {
			return err
		}
		summary.SkippedCrossBatch = d3

		transfers, err := q.CountActiveTransfersForBatch(ctx, batchPg)
		if err != nil {
			return fmt.Errorf("counting internal transfers: %w", err)
		}
		summary.InternalTransfers = int32(transfers)
		return nil
	})
	if err != nil {
		return Summary{}, fmt.Errorf("ingest: get_dedup_summary: %w", err)
	}
	return summary, nil
}

// ListSkippedRows returns every row dedup_skips holds for a batch, in file
// order.
func (s *Service) ListSkippedRows(ctx context.Context, userID, requestedOrgID, batchID uuid.UUID) ([]dedup.Skip, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return nil, err
	}

	var skips []dedup.Skip
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		skips, err = dedup.ListSkips(ctx, tx, batchID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("ingest: list_skipped_rows: %w", err)
	}
	return skips, nil
}

// ListInternalTransfers returns every active (undismissed) pair touching
// entityID, on either side.
func (s *Service) ListInternalTransfers(ctx context.Context, userID, requestedOrgID, entityID uuid.UUID) ([]dedup.Transfer, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return nil, err
	}

	var transfers []dedup.Transfer
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		transfers, err = dedup.ListActiveTransfers(ctx, tx, entityID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("ingest: list_internal_transfers: %w", err)
	}
	return transfers, nil
}

// DismissInternalTransfer returns a pair to the P&L (design D4: excluded on
// detection, reversible by dismissal).
func (s *Service) DismissInternalTransfer(ctx context.Context, userID, requestedOrgID, transferID uuid.UUID) (dedup.Transfer, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return dedup.Transfer{}, err
	}

	var transfer dedup.Transfer
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		transfer, err = dedup.Dismiss(ctx, tx, transferID, userID)
		return err
	})
	if err != nil {
		return dedup.Transfer{}, fmt.Errorf("ingest: dismiss_internal_transfer: %w", err)
	}
	return transfer, nil
}
