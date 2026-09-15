package ledger

import (
	"testing"

	"github.com/google/uuid"

	"github.com/MyauDev/vekst/core/internal/money"
)

// ToClassifyBatch is a pure mapper -- no database, no fixture pipeline
// needed to exercise it, only a Transaction with every field it reads set
// to something distinguishable.
func TestToClassifyBatch(t *testing.T) {
	txnID := uuid.New()
	accountID := uuid.New()
	txns := []Transaction{{
		ID:              txnID,
		AccountID:       accountID,
		SourceKind:      SourceKindBank,
		DescriptionNorm: "coffee shop",
		CounterpartyKey: "name:coffee-shop",
		Direction:       DirectionExpense,
		Amount:          money.Money{CurrencyCode: "NOK", MinorUnits: -500},
		RegulatedCode:   "332",
	}}

	got := ToClassifyBatch(txns)
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	c := got[0]

	// Design D7: the id crossing to the classifier is the database's own,
	// rendered as text -- never a surrogate index.
	if c.TransactionID != txnID.String() {
		t.Errorf("TransactionID = %q, want %q", c.TransactionID, txnID.String())
	}
	if c.AccountID != accountID.String() {
		t.Errorf("AccountID = %q, want %q", c.AccountID, accountID.String())
	}
	if c.SourceKind != SourceKindBank {
		t.Errorf("SourceKind = %q, want %q", c.SourceKind, SourceKindBank)
	}
	if c.DescriptionNorm != "coffee shop" {
		t.Errorf("DescriptionNorm = %q", c.DescriptionNorm)
	}
	if c.CounterpartyKey != "name:coffee-shop" {
		t.Errorf("CounterpartyKey = %q", c.CounterpartyKey)
	}
	if c.Direction != DirectionExpense {
		t.Errorf("Direction = %q, want %q", c.Direction, DirectionExpense)
	}
	if c.Amount != txns[0].Amount {
		t.Errorf("Amount = %+v, want %+v", c.Amount, txns[0].Amount)
	}
	if c.RegulatedCode != "332" {
		t.Errorf("RegulatedCode = %q", c.RegulatedCode)
	}
}

func TestToClassifyBatchOfMultipleRows(t *testing.T) {
	txns := []Transaction{
		{ID: uuid.New(), AccountID: uuid.New()},
		{ID: uuid.New(), AccountID: uuid.New()},
		{ID: uuid.New(), AccountID: uuid.New()},
	}
	got := ToClassifyBatch(txns)
	if len(got) != len(txns) {
		t.Fatalf("got %d entries, want %d", len(got), len(txns))
	}
	for i, c := range got {
		if c.TransactionID != txns[i].ID.String() {
			t.Errorf("entry %d: TransactionID = %q, want %q", i, c.TransactionID, txns[i].ID.String())
		}
	}
}
