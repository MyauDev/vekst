package ledger

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/money"
)

// Insert stores txns and returns them back with what the database assigned
// -- ID and CreatedAt. It takes a running transaction, never a pool
// (CLAUDE.md: db.InTx is the one transaction entry point), so a caller
// always owns the tenant context this runs under.
//
// Insert writes whatever DedupHash each Transaction already carries; it
// does not compute one. A caller builds a batch, calls DedupHash once per
// row with that row's occurrence within the batch, and only then calls
// Insert -- keeping the one field the schema is strictest about written
// down in one place (DedupHash's own doc comment).
func Insert(ctx context.Context, tx pgx.Tx, org db.OrgID, txns []Transaction) ([]Transaction, error) {
	q := gendb.New(tx)
	out := make([]Transaction, len(txns))
	for i, t := range txns {
		params, err := toInsertParams(org, t)
		if err != nil {
			return nil, fmt.Errorf("ledger: building transaction %d: %w", i, err)
		}
		row, err := q.InsertTransaction(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("ledger: inserting transaction %d: %w", i, err)
		}
		out[i], err = transactionFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("ledger: reading back transaction %d: %w", i, err)
		}
	}
	return out, nil
}

func toInsertParams(org db.OrgID, t Transaction) (gendb.InsertTransactionParams, error) {
	amountCurrency, err := currencyCheck(t.Amount)
	if err != nil {
		return gendb.InsertTransactionParams{}, err
	}

	params := gendb.InsertTransactionParams{
		OrgID:            pgtype.UUID{Bytes: org.UUID(), Valid: true},
		EntityID:         pgtype.UUID{Bytes: t.EntityID, Valid: true},
		AccountID:        pgtype.UUID{Bytes: t.AccountID, Valid: true},
		BatchID:          pgtype.UUID{Bytes: t.BatchID, Valid: true},
		SourceKind:       t.SourceKind,
		DocumentRef:      pgtype.Text{String: t.DocumentRef, Valid: t.DocumentRef != ""},
		PostingNo:        t.PostingNo,
		BookedOn:         pgtype.Date{Time: t.BookedOn, Valid: true},
		ValueOn:          pgtype.Date{Time: t.ValueOn, Valid: !t.ValueOn.IsZero()},
		AmountMinor:      t.Amount.MinorUnits,
		Currency:         amountCurrency,
		CounterpartyRaw:  t.CounterpartyRaw,
		CounterpartyKey:  t.CounterpartyKey,
		DescriptionRaw:   t.DescriptionRaw,
		DescriptionNorm:  t.DescriptionNorm,
		NormalizeVersion: t.NormalizeVersion,
		RegulatedCode:    t.RegulatedCode,
		BankRef:          t.BankRef,
		DedupHash:        t.DedupHash,
	}

	if t.FX != nil {
		rate, err := numericFromDecimalString(t.FX.Rate)
		if err != nil {
			return gendb.InsertTransactionParams{}, fmt.Errorf("fx_rate: %w", err)
		}
		baseCurrency, err := currencyCheck(t.FX.Base)
		if err != nil {
			return gendb.InsertTransactionParams{}, err
		}
		params.FxRate = rate
		params.FxRateOn = pgtype.Date{Time: t.FX.RateOn, Valid: true}
		params.BaseAmountMinor = pgtype.Int8{Int64: t.FX.Base.MinorUnits, Valid: true}
		params.BaseCurrency = pgtype.Text{String: baseCurrency, Valid: true}
	}

	return params, nil
}

func transactionFromRow(row gendb.Transaction) (Transaction, error) {
	t := Transaction{
		ID:               uuid.UUID(row.ID.Bytes),
		EntityID:         uuid.UUID(row.EntityID.Bytes),
		AccountID:        uuid.UUID(row.AccountID.Bytes),
		BatchID:          uuid.UUID(row.BatchID.Bytes),
		SourceKind:       row.SourceKind,
		DocumentRef:      row.DocumentRef.String,
		PostingNo:        row.PostingNo,
		BookedOn:         row.BookedOn.Time,
		Direction:        row.Direction,
		Amount:           money.Money{CurrencyCode: row.Currency, MinorUnits: row.AmountMinor},
		CounterpartyRaw:  row.CounterpartyRaw,
		CounterpartyKey:  row.CounterpartyKey,
		DescriptionRaw:   row.DescriptionRaw,
		DescriptionNorm:  row.DescriptionNorm,
		NormalizeVersion: row.NormalizeVersion,
		RegulatedCode:    row.RegulatedCode,
		BankRef:          row.BankRef,
		DedupHash:        row.DedupHash,
		CreatedAt:        row.CreatedAt.Time,
	}
	if row.ValueOn.Valid {
		t.ValueOn = row.ValueOn.Time
	}
	if row.FxRate.Valid {
		rate, err := decimalStringFromNumeric(row.FxRate)
		if err != nil {
			return Transaction{}, fmt.Errorf("fx_rate: %w", err)
		}
		t.FX = &FXConversion{
			Rate:   rate,
			RateOn: row.FxRateOn.Time,
			Base:   money.Money{CurrencyCode: row.BaseCurrency.String, MinorUnits: row.BaseAmountMinor.Int64},
		}
	}
	return t, nil
}

// currencyCheck validates m's currency code against the exponent table
// before it reaches a query parameter -- catching a typo here rather than
// as a row this schema's regex CHECK alone would still have accepted.
func currencyCheck(m money.Money) (string, error) {
	if _, ok := money.Exponent(m.CurrencyCode); !ok {
		return "", fmt.Errorf("%w: %q", money.ErrUnknownCurrency, m.CurrencyCode)
	}
	return m.CurrencyCode, nil
}
