package ledger

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MyauDev/vekst/core/internal/db"
)

func seedOneTransaction(t *testing.T, d *db.DB, org db.OrgID, entityID, account, batch uuid.UUID, sourceKind string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		out, err := Insert(ctx, tx, org, []Transaction{baseTransaction(entityID, account, batch, sourceKind)})
		if err != nil {
			return err
		}
		id = out[0].ID
		return nil
	})
	if err != nil {
		t.Fatalf("seeding a transaction: %v", err)
	}
	return id
}

// Task 4.7: append-only, as grants rather than as a comment. vekst_app
// cannot UPDATE a classification's category or DELETE a row; superseding
// one -- an insert plus a pointer -- works and leaves exactly one live row.
func TestClassificationsAreAppendOnly(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, SourceKindBank)
	leaf := testCategoryID(t, d, org, leafCategoryCode)
	txnID := seedOneTransaction(t, d, org, entityID, account, batch, SourceKindBank)

	var first Classification
	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		first, err = InsertClassification(ctx, tx, org, Classification{
			TransactionID: txnID, CategoryID: leaf, EngineLayer: EngineLayerL0,
			Confidence: 0.8, TaxonomyVersion: "v1", RulesetVersion: "v1", EngineVersion: "v1",
			NormalizeVersion: "v1",
		})
		return err
	})
	if err != nil {
		t.Fatalf("InsertClassification: %v", err)
	}

	// vekst_app holds no UPDATE on category_id, and no DELETE at all
	// (migration 007's REVOKE/GRANT, design D4) -- a column grant refuses
	// this before any row-level check runs.
	err = d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE classifications SET category_id = $2 WHERE id = $1`,
			pgtype.UUID{Bytes: first.ID, Valid: true}, pgtype.UUID{Bytes: leaf, Valid: true})
		return err
	})
	if code := pgCode(err); code != "42501" {
		t.Errorf("UPDATE category_id: SQLSTATE = %q (%v), want 42501 (insufficient_privilege)", code, err)
	}

	err = d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM classifications WHERE id = $1`, pgtype.UUID{Bytes: first.ID, Valid: true})
		return err
	})
	if code := pgCode(err); code != "42501" {
		t.Errorf("DELETE: SQLSTATE = %q (%v), want 42501 (insufficient_privilege)", code, err)
	}

	// Superseding: an insert plus a pointer, in one statement (see
	// SupersedeClassification's own query comment for why two statements
	// cannot do this against classifications_one_live_idx).
	var second Classification
	err = d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		second, err = SupersedeClassification(ctx, tx, org, first.ID, Classification{
			TransactionID: txnID, CategoryID: leaf, EngineLayer: EngineLayerHuman,
			Confidence: 1.0, TaxonomyVersion: "v1", RulesetVersion: "v1", EngineVersion: "v1",
			NormalizeVersion: "v1", DecidedBy: owner,
		})
		return err
	})
	if err != nil {
		t.Fatalf("superseding: %v", err)
	}

	var live Classification
	err = d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		live, err = CurrentClassification(ctx, tx, txnID)
		return err
	})
	if err != nil {
		t.Fatalf("CurrentClassification: %v", err)
	}
	if live.ID != second.ID {
		t.Errorf("the live classification is %s, want the superseding one %s", live.ID, second.ID)
	}
}

// Task 4.8: at most one live classification per transaction.
// classifications_one_live_idx is what makes "the current answer" a fact,
// not a query convention -- a second live row for the same transaction is
// a unique violation.
func TestOnlyOneLiveClassificationPerTransaction(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, SourceKindBank)
	leaf := testCategoryID(t, d, org, leafCategoryCode)
	txnID := seedOneTransaction(t, d, org, entityID, account, batch, SourceKindBank)

	classify := func() error {
		return d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
			_, err := InsertClassification(ctx, tx, org, Classification{
				TransactionID: txnID, CategoryID: leaf, EngineLayer: EngineLayerL0,
				Confidence: 0.8, TaxonomyVersion: "v1", RulesetVersion: "v1", EngineVersion: "v1",
				NormalizeVersion: "v1",
			})
			return err
		})
	}

	if err := classify(); err != nil {
		t.Fatalf("first classification: %v", err)
	}
	err := classify()
	if code := pgCode(err); code != "23505" {
		t.Errorf("a second live classification: SQLSTATE = %q (%v), want 23505", code, err)
	}
}

// Task 4.10: a classification cannot target a section or a computed line,
// with the same error migration 006's rules_category_is_visible raises for
// classification_rules -- reused unchanged for classifications.
func TestAClassificationCannotTargetASectionOrComputedLine(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, SourceKindBank)
	notClassifiable := testCategoryID(t, d, org, sectionOrComputedCategoryCode)
	txnID := seedOneTransaction(t, d, org, entityID, account, batch, SourceKindBank)

	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := InsertClassification(ctx, tx, org, Classification{
			TransactionID: txnID, CategoryID: notClassifiable, EngineLayer: EngineLayerL0,
			Confidence: 0.8, TaxonomyVersion: "v1", RulesetVersion: "v1", EngineVersion: "v1",
			NormalizeVersion: "v1",
		})
		return err
	})
	if code := pgCode(err); code != "23514" {
		t.Errorf("classifying a section/computed category: SQLSTATE = %q (%v), want 23514", code, err)
	}
}
