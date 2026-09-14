package dedup

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/ledger"
	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/normalize"
)

// insertTxn is testTransaction's more general cousin: it takes a whole
// ledger.Transaction (so these tests can set FX, source_kind and booked_on
// precisely) rather than just an amount.
func insertTxn(t *testing.T, d *db.DB, org db.OrgID, txn ledger.Transaction) ledger.Transaction {
	t.Helper()
	if txn.DescriptionNorm == "" {
		txn.DescriptionNorm = "test payment"
	}
	if txn.NormalizeVersion == "" {
		txn.NormalizeVersion = normalize.Version
	}
	if txn.DedupHash == "" {
		txn.DedupHash = ledger.DedupHash(txn, 1)
	}
	var out []ledger.Transaction
	err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = ledger.Insert(ctx, tx, org, []ledger.Transaction{txn})
		return err
	})
	if err != nil {
		t.Fatalf("seeding transaction: %v", err)
	}
	return out[0]
}

// Task 6.10: transfer detection. An out and an in, equal, two days apart,
// different accounts, one organisation, produce one pair.
func TestTransferDetection(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account1 := testAccount(t, d, org, entityID, "NOK")
	account2 := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, ledger.SourceKindBank)

	out := insertTxn(t, d, org, ledger.Transaction{
		EntityID: entityID, AccountID: account1, BatchID: batch, SourceKind: ledger.SourceKindBank,
		BookedOn: date(2026, 1, 10), Direction: ledger.DirectionExpense,
		Amount: money.Money{CurrencyCode: "NOK", MinorUnits: -10000},
	})

	var pair *Transfer
	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		in := insertViaTx(t, tx, org, ledger.Transaction{
			EntityID: entityID, AccountID: account2, BatchID: batch, SourceKind: ledger.SourceKindBank,
			BookedOn: date(2026, 1, 12), Direction: ledger.DirectionIncome,
			Amount: money.Money{CurrencyCode: "NOK", MinorUnits: 10000},
		})
		var err error
		pair, err = DetectAndPair(ctx, tx, org, entityID, in)
		return err
	})
	if err != nil {
		t.Fatalf("DetectAndPair: %v", err)
	}
	if pair == nil {
		t.Fatal("no pair detected for an equal, opposite, two-day-apart, different-account transfer")
	}
	if pair.OutTxnID != out.ID {
		t.Errorf("OutTxnID = %s, want %s", pair.OutTxnID, out.ID)
	}
}

// Task 6.11: across currencies. A PLN-to-EUR transfer between the
// customer's own accounts pairs on the base amount, and the pair is
// unchanged by a later import carrying a different rate -- pairs are
// written once and never re-paired (design D5's own risk table).
func TestTransferAcrossCurrencies(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner) // base currency NOK
	plnAccount := testAccount(t, d, org, entityID, "PLN")
	eurAccount := testAccount(t, d, org, entityID, "EUR")
	batch := testBatch(t, d, org, entityID, ledger.SourceKindBank)

	// Money left the PLN account, converted to NOK 10000 at whatever rate
	// applied that day.
	out := insertTxn(t, d, org, ledger.Transaction{
		EntityID: entityID, AccountID: plnAccount, BatchID: batch, SourceKind: ledger.SourceKindBank,
		BookedOn: date(2026, 1, 10), Direction: ledger.DirectionExpense,
		Amount: money.Money{CurrencyCode: "PLN", MinorUnits: -400000},
		FX: &ledger.FXConversion{
			Rate: "0.4000000000", RateOn: date(2026, 1, 10),
			Base: money.Money{CurrencyCode: "NOK", MinorUnits: -10000},
		},
	})

	var pair *Transfer
	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		in := insertViaTx(t, tx, org, ledger.Transaction{
			EntityID: entityID, AccountID: eurAccount, BatchID: batch, SourceKind: ledger.SourceKindBank,
			BookedOn: date(2026, 1, 11), Direction: ledger.DirectionIncome,
			Amount: money.Money{CurrencyCode: "EUR", MinorUnits: 90000},
			FX: &ledger.FXConversion{
				// A different rate on a different day: 10000 NOK all the
				// same, which is the point -- the pairing reads the base
				// amount each row already carries, never recomputes one.
				Rate: "0.1111111111", RateOn: date(2026, 1, 11),
				Base: money.Money{CurrencyCode: "NOK", MinorUnits: 10000},
			},
		})
		var err error
		pair, err = DetectAndPair(ctx, tx, org, entityID, in)
		return err
	})
	if err != nil {
		t.Fatalf("DetectAndPair: %v", err)
	}
	if pair == nil {
		t.Fatal("a PLN-to-EUR transfer did not pair on its base amount")
	}
	if pair.OutTxnID != out.ID {
		t.Errorf("OutTxnID = %s, want %s", pair.OutTxnID, out.ID)
	}
}

// Task 6.12: within one source kind. A ledger row and a bank row are never
// paired as an internal transfer -- that is D4's job, and D4 is Product.
func TestTransferNeverCrossesSourceKind(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account1 := testAccount(t, d, org, entityID, "NOK")
	account2 := testAccount(t, d, org, entityID, "NOK")
	bankBatch := testBatch(t, d, org, entityID, ledger.SourceKindBank)
	ledgerBatch := testBatch(t, d, org, entityID, ledger.SourceKindLedger)

	insertTxn(t, d, org, ledger.Transaction{
		EntityID: entityID, AccountID: account1, BatchID: bankBatch, SourceKind: ledger.SourceKindBank,
		BookedOn: date(2026, 1, 10), Direction: ledger.DirectionExpense,
		Amount: money.Money{CurrencyCode: "NOK", MinorUnits: -10000},
	})

	var pair *Transfer
	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		in := insertViaTx(t, tx, org, ledger.Transaction{
			EntityID: entityID, AccountID: account2, BatchID: ledgerBatch, SourceKind: ledger.SourceKindLedger,
			DocumentRef: "DOC-1", PostingNo: 1,
			BookedOn: date(2026, 1, 11), Direction: ledger.DirectionIncome,
			Amount: money.Money{CurrencyCode: "NOK", MinorUnits: 10000},
		})
		var err error
		pair, err = DetectAndPair(ctx, tx, org, entityID, in)
		return err
	})
	if err != nil {
		t.Fatalf("DetectAndPair: %v", err)
	}
	if pair != nil {
		t.Errorf("a ledger row paired with a bank row: %+v", pair)
	}
}

// Task 6.13: deterministic. Three equal transfers in one week produce the
// same pairing whatever order the rows arrive in.
func TestTransferPairingIsDeterministic(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	outAccount := testAccount(t, d, org, entityID, "NOK")
	inAccount := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, ledger.SourceKindBank)

	// Three "out" legs on three different days, all -10000, all in the same
	// account, so any one of them is a legal candidate for an "in" leg
	// landing in the middle of the window.
	var outs []ledger.Transaction
	for _, day := range []int{9, 10, 11} {
		outs = append(outs, insertTxn(t, d, org, ledger.Transaction{
			EntityID: entityID, AccountID: outAccount, BatchID: batch, SourceKind: ledger.SourceKindBank,
			BookedOn: date(2026, 1, day), Direction: ledger.DirectionExpense,
			Amount: money.Money{CurrencyCode: "NOK", MinorUnits: -10000},
		}))
	}

	var pair *Transfer
	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		in := insertViaTx(t, tx, org, ledger.Transaction{
			EntityID: entityID, AccountID: inAccount, BatchID: batch, SourceKind: ledger.SourceKindBank,
			BookedOn: date(2026, 1, 10), Direction: ledger.DirectionIncome,
			Amount: money.Money{CurrencyCode: "NOK", MinorUnits: 10000},
		})
		var err error
		pair, err = DetectAndPair(ctx, tx, org, entityID, in)
		return err
	})
	if err != nil {
		t.Fatalf("DetectAndPair: %v", err)
	}
	if pair == nil {
		t.Fatal("no pair detected")
	}
	// Nearest by date (design D5): day 10 is 0 days away, days 9 and 11 are
	// each 1 day away -- day 10's own leg is the unambiguous nearest one.
	if pair.OutTxnID != outs[1].ID {
		t.Errorf("OutTxnID = %s, want the nearest-by-date candidate %s (booked day 10)", pair.OutTxnID, outs[1].ID)
	}
}

// insertViaTx inserts one transaction using a transaction already open --
// ledger.Insert takes pgx.Tx directly, so this is a thin wrapper for the
// tests above that need to seed the "in" leg in the same transaction
// DetectAndPair runs in.
func insertViaTx(t *testing.T, tx pgx.Tx, org db.OrgID, txn ledger.Transaction) ledger.Transaction {
	t.Helper()
	if txn.DescriptionNorm == "" {
		txn.DescriptionNorm = "test payment"
	}
	if txn.NormalizeVersion == "" {
		txn.NormalizeVersion = normalize.Version
	}
	if txn.DedupHash == "" {
		txn.DedupHash = ledger.DedupHash(txn, 1)
	}
	out, err := ledger.Insert(context.Background(), tx, org, []ledger.Transaction{txn})
	if err != nil {
		t.Fatalf("seeding transaction inside an open tx: %v", err)
	}
	return out[0]
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
