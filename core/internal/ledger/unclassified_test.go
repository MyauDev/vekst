package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// "Which rows has nobody answered for" is the question the classifier's worker
// asks, the review queue asks, and every report asks about its own buckets.
// Three readers, one definition, and this is where that definition lives.
//
// All three properties below were wrong when change 3.4 came to page through
// this, and none of them was visible without actually doing so.

// A batch's rows, and only that batch's.
//
// Two imports can be in flight at once -- a customer uploading January and
// February within a minute of each other is ordinary -- and a read across the
// organisation would hand each job the other's rows. Both would classify them,
// both would count them, and the counts on two runs would each be right about a
// batch neither of them describes.
func TestUnclassifiedTransactionsAreScopedToOneBatch(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")

	mine := testBatch(t, d, org, entityID, SourceKindBank)
	theirs := testBatch(t, d, org, entityID, SourceKindBank)

	plant := func(batch uuid.UUID, n int) {
		t.Helper()
		err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
			for i := range n {
				txn := baseTransaction(entityID, account, batch, SourceKindBank)
				txn.LineNo = int32(i + 1)
				txn.BookedOn = time.Date(2026, 3, i+1, 0, 0, 0, 0, time.UTC)
				txn.DedupHash = uuid.NewString()
				if _, err := Insert(ctx, tx, org, []Transaction{txn}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	plant(mine, 3)
	plant(theirs, 2)

	var page []Transaction
	if err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		page, err = UnclassifiedTransactions(ctx, tx, mine, Cursor{}, 50)
		return err
	}); err != nil {
		t.Fatalf("UnclassifiedTransactions: %v", err)
	}

	if len(page) != 3 {
		t.Fatalf("%d rows, want this batch's 3 and not the other batch's 2 as well", len(page))
	}
	for _, r := range page {
		if r.BatchID != mine {
			t.Errorf("row %s belongs to batch %s", r.ID, r.BatchID)
		}
	}
}

// The cursor, and why it is not an offset and not a self-advancing read.
//
// The original query relied on rows leaving the result as they were classified,
// so the next call returned the next page. That is true only while every row
// gets classified -- and a confidence threshold exists precisely so that some do
// not. One unanswered row and the caller reads the same page until something
// kills it.
func TestUnclassifiedTransactionsPageWithoutSkippingOrRepeating(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, SourceKindBank)

	const total = 7
	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		for i := range total {
			txn := baseTransaction(entityID, account, batch, SourceKindBank)
			txn.LineNo = int32(i + 1)
			// Every row on one day, so the date orders nothing and the tiebreak
			// does all the work. A cursor on the date alone would skip or
			// repeat here, and a fixture spreading them out would not notice.
			txn.BookedOn = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
			txn.DedupHash = uuid.NewString()
			if _, err := Insert(ctx, tx, org, []Transaction{txn}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}

	// Nothing is ever classified, so nothing leaves the result.
	seen := map[uuid.UUID]int{}
	var cursor Cursor
	var pages int
	for {
		var page []Transaction
		if err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			page, err = UnclassifiedTransactions(ctx, tx, batch, cursor, 3)
			return err
		}); err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		if len(page) == 0 {
			break
		}
		pages++
		if pages > total {
			t.Fatal("the read never reached the end: it is not advancing, which is what " +
				"a below-threshold row does to a self-advancing query")
		}
		for _, r := range page {
			seen[r.ID]++
		}
		last := page[len(page)-1]
		cursor = Cursor{BookedOn: last.BookedOn, ID: last.ID}
	}

	if len(seen) != total {
		t.Errorf("%d distinct rows over %d pages, want %d", len(seen), pages, total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("row %s came back %d times", id, n)
		}
	}
	if pages != 3 {
		t.Errorf("%d pages for %d rows at 3 a page, want 3", pages, total)
	}
}

// A retraction puts a row back, and a supersession does not.
//
// They are different events. A correction names the classification that
// replaced it, so the row has an answer -- a different one. An undone review
// decision has no successor to point at, so the row has none at all. A
// definition of "unclassified" that knows only about supersession leaves a
// retracted row invisible to the classifier while the review queue and every
// report count it as unanswered: stuck, permanently, in the one state nothing
// acts on.
func TestARetractedClassificationMakesARowUnclassifiedAgain(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, SourceKindBank)
	category := testCategoryID(t, d, org, "0101")

	var answered, retracted, superseded Transaction
	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		for i, into := range []*Transaction{&answered, &retracted, &superseded} {
			txn := baseTransaction(entityID, account, batch, SourceKindBank)
			txn.LineNo = int32(i + 1)
			txn.BookedOn = time.Date(2026, 3, i+1, 0, 0, 0, 0, time.UTC)
			txn.DedupHash = uuid.NewString()
			out, err := Insert(ctx, tx, org, []Transaction{txn})
			if err != nil {
				return err
			}
			*into = out[0]
		}

		c := Classification{
			CategoryID: category, EngineLayer: "L1", Confidence: 0.95,
			TaxonomyVersion: "v1", RulesetVersion: "v1",
			EngineVersion: "engine-1", NormalizeVersion: "v1",
		}
		for _, t := range []Transaction{answered, retracted, superseded} {
			c.TransactionID = t.ID
			if _, err := InsertClassification(ctx, tx, org, c); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}

	err = d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		// One withdrawn with nothing in its place.
		if _, err := tx.Exec(ctx, `
			UPDATE classifications SET retracted_at = now(), retracted_by = $2
			 WHERE transaction_id = $1`,
			pgtype.UUID{Bytes: retracted.ID, Valid: true},
			pgtype.UUID{Bytes: owner, Valid: true}); err != nil {
			return err
		}
		// And one corrected, which leaves an answer behind.
		old, err := CurrentClassification(ctx, tx, superseded.ID)
		if err != nil {
			return err
		}
		_, err = SupersedeClassification(ctx, tx, org, old.ID, Classification{
			TransactionID: superseded.ID, CategoryID: category,
			EngineLayer: "L1", Confidence: 0.95,
			TaxonomyVersion: "v1", RulesetVersion: "v1",
			EngineVersion: "engine-2", NormalizeVersion: "v1",
		})
		return err
	})
	if err != nil {
		t.Fatalf("retracting and superseding: %v", err)
	}

	var page []Transaction
	if err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		page, err = UnclassifiedTransactions(ctx, tx, batch, Cursor{}, 50)
		return err
	}); err != nil {
		t.Fatalf("UnclassifiedTransactions: %v", err)
	}

	if len(page) != 1 {
		t.Fatalf("%d unclassified rows, want 1 (the retracted one)", len(page))
	}
	if page[0].ID != retracted.ID {
		switch page[0].ID {
		case answered.ID:
			t.Error("a row with a live classification came back as unclassified")
		case superseded.ID:
			t.Error("a corrected row came back as unclassified: a supersession names the " +
				"classification that replaced it, so the row has an answer")
		}
	}
}

// A page of nothing is not a page.
func TestUnclassifiedTransactionsRefusesAnEmptyLimit(t *testing.T) {
	d := testDB(t)
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	batch := testBatch(t, d, org, entityID, SourceKindBank)

	for _, limit := range []int32{0, -1} {
		err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
			_, err := UnclassifiedTransactions(ctx, tx, batch, Cursor{}, limit)
			return err
		})
		if err == nil {
			t.Errorf("a limit of %d was accepted; a caller looping on an empty page never "+
				"terminates and never says why", limit)
		}
	}
}
