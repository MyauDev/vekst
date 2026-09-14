package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/MyauDev/vekst/core/internal/money"
)

// Task 4.11: DedupHash. The same content at the same occurrence hashes the
// same; two rows differing in one field differ; and two genuinely identical
// payments in one batch -- different occurrences -- are both stored rather
// than one being rejected (design D5).
func TestDedupHash(t *testing.T) {
	base := Transaction{
		AccountID:       uuid.New(),
		BookedOn:        mustDate(t, "2026-01-15"),
		Amount:          money.Money{CurrencyCode: "NOK", MinorUnits: 500},
		DescriptionNorm: "coffee shop",
		BankRef:         "",
		DocumentRef:     "",
		PostingNo:       0,
	}

	t.Run("same content, same occurrence, same hash", func(t *testing.T) {
		first := DedupHash(base, 1)
		second := DedupHash(base, 1)
		if first != second {
			t.Error("identical Transaction and occurrence produced different hashes")
		}
	})

	t.Run("differing in one field, different hash", func(t *testing.T) {
		other := base
		other.DescriptionNorm = "coffee shop 2"
		if DedupHash(base, 1) == DedupHash(other, 1) {
			t.Error("a different description_norm produced the same hash")
		}
	})

	t.Run("same content, different occurrence, different hash", func(t *testing.T) {
		if DedupHash(base, 1) == DedupHash(base, 2) {
			t.Error("occurrence 1 and occurrence 2 of the same content hashed the same")
		}
	})
}

// The other half of design D5: two coffees bought on the same day, for the
// same amount, with the same wording and no bank reference are two real
// rows -- occurrence 1 and 2 -- and both are stored, because their hashes
// differ.
func TestTwoIdenticalPaymentsInOneBatchAreBothStored(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, SourceKindBank)

	txn := baseTransaction(entityID, account, batch, SourceKindBank)
	txn.DescriptionNorm = "coffee shop"
	txn.BankRef = ""
	txn.DocumentRef = ""
	txn.PostingNo = 0

	first := txn
	first.DedupHash = DedupHash(first, 1)
	second := txn
	second.DedupHash = DedupHash(second, 2)

	var out []Transaction
	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = Insert(ctx, tx, org, []Transaction{first, second})
		return err
	})
	if err != nil {
		t.Fatalf("two identical payments, different occurrences, were refused: %v", err)
	}
	if len(out) != 2 || out[0].ID == out[1].ID {
		t.Fatalf("got %d row(s) back, want two distinct rows", len(out))
	}

	// transactions_dedup_idx is deliberately not unique (migration 007,
	// corrected by add-dedup's own task 0.4): the same content at the same
	// occurrence -- a re-import of the same file -- is not refused by the
	// database at all. Recognising it and skipping it is add-dedup's own
	// D2/D3 job, in application code, precisely because a hard constraint
	// here cannot tell a genuine re-import from an unrelated coincidence
	// (design D3) and cannot record what it refused.
	replay := txn
	replay.DedupHash = DedupHash(replay, 1)
	err = d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := Insert(ctx, tx, org, []Transaction{replay})
		return err
	})
	if err != nil {
		t.Errorf("Insert refused occurrence 1's content a second time, but the database is not what should: %v", err)
	}
}

func mustDate(t *testing.T, s string) (d time.Time) {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return d
}
