package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/money"
)

// Task 4.3: the grain. A bank row (DocumentRef "", PostingNo 0) is
// accepted; one with a document reference but PostingNo 0 is rejected; a
// ledger document's five postings, sharing one DocumentRef and numbered
// 1..5, are all accepted.
func TestGrainConstraintHolds(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")

	t.Run("a bank payment has no document_ref and posting_no zero", func(t *testing.T) {
		batch := testBatch(t, d, org, entityID, SourceKindBank)
		txn := baseTransaction(entityID, account, batch, SourceKindBank)
		err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
			_, err := Insert(ctx, tx, org, []Transaction{txn})
			return err
		})
		if err != nil {
			t.Fatalf("a well-formed bank payment was refused: %v", err)
		}
	})

	t.Run("a document_ref with posting_no zero is refused", func(t *testing.T) {
		batch := testBatch(t, d, org, entityID, SourceKindBank)
		txn := baseTransaction(entityID, account, batch, SourceKindBank)
		txn.DocumentRef = "DOC-1"
		txn.PostingNo = 0
		err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
			_, err := Insert(ctx, tx, org, []Transaction{txn})
			return err
		})
		if code := pgCode(err); code != "23514" {
			t.Errorf("document_ref set with posting_no 0: SQLSTATE = %q (%v), want 23514", code, err)
		}
	})

	t.Run("a ledger document's five postings share one document_ref", func(t *testing.T) {
		batch := testBatch(t, d, org, entityID, SourceKindLedger)
		txns := make([]Transaction, 5)
		for i := range txns {
			txn := baseTransaction(entityID, account, batch, SourceKindLedger)
			txn.DocumentRef = "DOC-42"
			txn.PostingNo = int32(i + 1)
			txn.DedupHash = uuid.NewString()
			txns[i] = txn
		}
		err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
			out, err := Insert(ctx, tx, org, txns)
			if err != nil {
				return err
			}
			if len(out) != 5 {
				t.Errorf("got %d rows back, want 5", len(out))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("five postings of one document were refused: %v", err)
		}
	})
}

// Task 4.4: non-base-currency. A JPY row (exponent 0) and a KWD row
// (exponent 3) round-trip through Go with no rounding, and their base
// amounts are the ones stored, not ones this package derives.
func TestNonBaseCurrencyRoundTripsWithNoRounding(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner) // base currency NOK
	batch := testBatch(t, d, org, entityID, SourceKindBank)

	cases := []struct {
		name     string
		currency string
		minor    int64
		rate     string
		baseNok  int64
	}{
		{"JPY, exponent 0", "JPY", 1_000_000, "6.5432100000", 65432},
		{"KWD, exponent 3", "KWD", 1_500, "31.0000000000", 46500},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := testAccount(t, d, org, entityID, tc.currency)
			txn := baseTransaction(entityID, account, batch, SourceKindBank)
			txn.Amount = money.Money{CurrencyCode: tc.currency, MinorUnits: tc.minor}
			txn.FX = &FXConversion{
				Rate:   tc.rate,
				RateOn: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				Base:   money.Money{CurrencyCode: "NOK", MinorUnits: tc.baseNok},
			}
			txn.DedupHash = uuid.NewString()

			var stored Transaction
			err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
				out, err := Insert(ctx, tx, org, []Transaction{txn})
				if err != nil {
					return err
				}
				stored = out[0]
				return nil
			})
			if err != nil {
				t.Fatalf("Insert: %v", err)
			}

			if stored.Amount.MinorUnits != tc.minor || stored.Amount.CurrencyCode != tc.currency {
				t.Errorf("Amount = %+v, want %s %d", stored.Amount, tc.currency, tc.minor)
			}
			if stored.FX == nil {
				t.Fatal("FX is nil after inserting a converted row")
			}
			if stored.FX.Rate != tc.rate {
				t.Errorf("FX.Rate = %q, want %q -- no rounding", stored.FX.Rate, tc.rate)
			}
			if stored.FX.Base.MinorUnits != tc.baseNok || stored.FX.Base.CurrencyCode != "NOK" {
				t.Errorf("FX.Base = %+v, want NOK %d -- stored, not derived", stored.FX.Base, tc.baseNok)
			}
			if !stored.FX.RateOn.Equal(txn.FX.RateOn) {
				t.Errorf("FX.RateOn = %v, want %v", stored.FX.RateOn, txn.FX.RateOn)
			}
		})
	}
}

// Task 4.6: FX is all-or-nothing. Four nullable columns admit sixteen
// states; exactly two mean anything (all null, or all four set), so the
// other fourteen partial ones are each refused. (Not fifteen: 2**4 - 2 =
// 14, a count the proposal's own design doc first wrote down before
// migration 007's inline comment corrected it -- corrected here too.)
func TestFXIsAllOrNothing(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, SourceKindBank)

	rate, err := numericFromDecimalString("1.2300000000")
	if err != nil {
		t.Fatalf("numericFromDecimalString: %v", err)
	}
	rateOn := pgtype.Date{Time: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), Valid: true}
	baseAmount := pgtype.Int8{Int64: 1500, Valid: true}
	baseCurrency := pgtype.Text{String: "NOK", Valid: true}

	full := [4]bool{true, true, true, true}
	rejected, accepted := 0, 0
	for mask := 0; mask < 16; mask++ {
		set := [4]bool{mask&1 != 0, mask&2 != 0, mask&4 != 0, mask&8 != 0}

		// The transaction's own currency is EUR, not NOK: the all-set state
		// converts to base_currency NOK, and txn_fx_is_a_conversion refuses
		// converting a currency into itself.
		txn := baseTransaction(entityID, account, batch, SourceKindBank)
		txn.Amount = money.Money{CurrencyCode: "EUR", MinorUnits: txn.Amount.MinorUnits}
		params, err := toInsertParams(org, txn)
		if err != nil {
			t.Fatalf("toInsertParams: %v", err)
		}
		params.DedupHash = uuid.NewString()
		if set[0] {
			params.FxRate = rate
		}
		if set[1] {
			params.FxRateOn = rateOn
		}
		if set[2] {
			params.BaseAmountMinor = baseAmount
		}
		if set[3] {
			params.BaseCurrency = baseCurrency
		}

		err = d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
			_, err := gendb.New(tx).InsertTransaction(ctx, params)
			return err
		})

		wantOK := set == [4]bool{} || set == full
		if wantOK {
			accepted++
			if err != nil {
				t.Errorf("mask %04b (all-or-nothing state): Insert failed: %v", mask, err)
			}
		} else {
			rejected++
			if code := pgCode(err); code != "23514" {
				t.Errorf("mask %04b (partial FX state): SQLSTATE = %q (%v), want 23514", mask, code, err)
			}
		}
	}
	if accepted != 2 || rejected != 14 {
		t.Fatalf("accepted %d, rejected %d out of 16 -- want 2 and 14", accepted, rejected)
	}
}

// Task 4.9: source_kind cannot drift from its batch's. migration 007's own
// constraint trigger raises 23514 the moment a row disagrees with the
// batch it names.
func TestSourceKindCannotDriftFromItsBatch(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")
	ledgerBatch := testBatch(t, d, org, entityID, SourceKindLedger)

	txn := baseTransaction(entityID, account, ledgerBatch, SourceKindBank) // drift: batch is ledger
	txn.DocumentRef = "DOC-1"
	txn.PostingNo = 1

	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := Insert(ctx, tx, org, []Transaction{txn})
		return err
	})
	if code := pgCode(err); code != "23514" {
		t.Errorf("a bank row on a ledger batch: SQLSTATE = %q (%v), want 23514", code, err)
	}
}

// Provenance is complete or the row is not written.
//
// `line_no` is the line of the original file, and a transaction that carries
// none cannot be traced back to the row a customer is looking at in their own
// spreadsheet. Migration 015 refuses it with a CHECK; this refuses it here,
// with a sentence, at the boundary that dropped it -- the persist path copied
// every other field of a parsed row across and not this one, and nothing
// noticed for two changes.
func TestATransactionWithNoLineNumberIsRefused(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, SourceKindBank)

	for name, lineNo := range map[string]int32{
		"unset":    0,
		"negative": -1,
	} {
		t.Run(name, func(t *testing.T) {
			txn := baseTransaction(entityID, account, batch, SourceKindBank)
			txn.LineNo = lineNo

			err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
				_, err := Insert(ctx, tx, org, []Transaction{txn})
				return err
			})
			if err == nil {
				t.Fatal("a transaction with no line number was stored; a sentinel in that " +
					"column is a number a customer reads as a line in their own file")
			}
		})
	}

	// And the line number survives the round trip, because refusing a bad one
	// is worth nothing if a good one is dropped.
	t.Run("a line number round-trips", func(t *testing.T) {
		txn := baseTransaction(entityID, account, batch, SourceKindBank)
		txn.LineNo = 4212
		txn.DedupHash = uuid.NewString()

		var stored Transaction
		err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
			out, err := Insert(ctx, tx, org, []Transaction{txn})
			if err != nil {
				return err
			}
			stored = out[0]
			return nil
		})
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}
		if stored.LineNo != 4212 {
			t.Errorf("line_no came back as %d, want 4212", stored.LineNo)
		}
		if stored.BatchID != batch {
			t.Errorf("batch came back as %s, want %s -- the pair is the provenance and "+
				"either alone is half an answer", stored.BatchID, batch)
		}
	})
}
