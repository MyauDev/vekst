package ingest

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/money"
)

// BatchRow is one transaction a batch persisted, with the classification it
// carries right now -- `report.DrillRow` in miniature, for the same reason
// `ImportedRow` in import.proto is `DrillTransaction` in miniature: a person
// opening a batch to see what happened to their file wants the same
// provenance a report's drill-down shows, read from the opposite direction.
type BatchRow struct {
	ID       uuid.UUID
	BookedOn string // YYYY-MM-DD
	LineNo   int32
	// The grain: 0 with no DocumentRef for a bank payment, 1..n for the
	// postings of one ledger document.
	PostingNo   int32
	DocumentRef string

	Amount money.Money
	// Zero-valued when the row needed no conversion -- "no conversion
	// happened" is a different claim from "converted at one".
	BaseAmount money.Money

	CounterpartyRaw string
	Description     string
	RegulatedCode   string
	SourceKind      string

	// The live classification. Empty on a row the review queue has not
	// reached yet -- which is the fact that put it there, not a missing
	// value.
	CategoryCode  string
	CategoryName  string
	EngineLayer   string
	Evidence      string
	Confidence    float64
	HasConfidence bool
}

// ListBatchTransactions returns every row a batch persisted, in file order,
// each with whatever classification it carries right now.
func (s *Service) ListBatchTransactions(ctx context.Context, userID, requestedOrgID, batchID uuid.UUID) ([]BatchRow, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return nil, err
	}

	var out []BatchRow
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)
		rows, err := q.BatchTransactions(ctx, pgtype.UUID{Bytes: batchID, Valid: true})
		if err != nil {
			return fmt.Errorf("reading the batch's transactions: %w", err)
		}

		for _, r := range rows {
			row := BatchRow{
				ID:              uuid.UUID(r.ID.Bytes),
				BookedOn:        r.BookedOn.Time.Format("2006-01-02"),
				LineNo:          r.LineNo,
				PostingNo:       r.PostingNo,
				DocumentRef:     r.DocumentRef.String,
				Amount:          money.Money{CurrencyCode: r.Currency, MinorUnits: r.AmountMinor},
				CounterpartyRaw: r.CounterpartyRaw,
				Description:     r.DescriptionRaw,
				RegulatedCode:   r.RegulatedCode,
				SourceKind:      r.SourceKind,
				CategoryCode:    r.CategoryCode,
				CategoryName:    r.CategoryName,
				EngineLayer:     r.EngineLayer,
				Evidence:        r.Evidence,
			}
			if r.BaseAmountMinor.Valid && r.BaseCurrency.Valid {
				row.BaseAmount = money.Money{
					CurrencyCode: r.BaseCurrency.String,
					MinorUnits:   r.BaseAmountMinor.Int64,
				}
			}
			if r.Confidence.Valid {
				v, err := r.Confidence.Float64Value()
				if err != nil {
					return fmt.Errorf("reading confidence: %w", err)
				}
				row.Confidence, row.HasConfidence = v.Float64, true
			}
			out = append(out, row)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ingest: list_batch_transactions: %w", err)
	}
	return out, nil
}
