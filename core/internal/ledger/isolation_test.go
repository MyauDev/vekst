package ledger

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Task 4.1: cross-tenant isolation on transactions. A cannot read, update
// or delete B's rows, and the outcome is indistinguishable from the row not
// existing -- row-level security, not a query that happens to filter.
func TestTransactionCrossTenantIsolation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	ownerA, ownerB := testUser(t, d), testUser(t, d)
	orgA, entityA := testOrgAndEntity(t, d, ownerA)
	orgB, _ := testOrgAndEntity(t, d, ownerB)

	accountA := testAccount(t, d, orgA, entityA, "NOK")
	batchA := testBatch(t, d, orgA, entityA, SourceKindBank)

	var txnID uuid.UUID
	err := d.InTx(ctx, orgA, func(ctx context.Context, tx pgx.Tx) error {
		out, err := Insert(ctx, tx, orgA, []Transaction{baseTransaction(entityA, accountA, batchA, SourceKindBank)})
		if err != nil {
			return err
		}
		txnID = out[0].ID
		return nil
	})
	if err != nil {
		t.Fatalf("seeding A's transaction: %v", err)
	}

	// B reads, updates and deletes under its own tenant context: A's row
	// does not exist as far as B's transaction is concerned -- the policy
	// is what refuses this, not a query predicate that merely omitted it.
	err = d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		var got pgtype.UUID
		return tx.QueryRow(ctx, `SELECT id FROM transactions WHERE id = $1`,
			pgtype.UUID{Bytes: txnID, Valid: true}).Scan(&got)
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("B reading A's transaction = %v, want pgx.ErrNoRows", err)
	}

	err = d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE transactions SET description_raw = 'tampered' WHERE id = $1`,
			pgtype.UUID{Bytes: txnID, Valid: true})
		return err
	})
	if err != nil {
		t.Fatalf("B's UPDATE against A's transaction errored rather than affecting zero rows: %v", err)
	}

	err = d.InTx(ctx, orgA, func(ctx context.Context, tx pgx.Tx) error {
		var descr string
		return tx.QueryRow(ctx, `SELECT description_raw FROM transactions WHERE id = $1`,
			pgtype.UUID{Bytes: txnID, Valid: true}).Scan(&descr)
	})
	if err != nil {
		t.Fatalf("re-reading A's transaction as A: %v", err)
	}
}

// Task 4.2: cross-tenant isolation on classifications, including that A
// cannot classify B's transaction -- the composite foreign key
// (org_id, transaction_id) has nothing in org A to point at.
func TestClassificationCrossTenantIsolation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	ownerA, ownerB := testUser(t, d), testUser(t, d)
	orgA, entityA := testOrgAndEntity(t, d, ownerA)
	orgB, entityB := testOrgAndEntity(t, d, ownerB)

	accountA := testAccount(t, d, orgA, entityA, "NOK")
	batchA := testBatch(t, d, orgA, entityA, SourceKindBank)
	accountB := testAccount(t, d, orgB, entityB, "NOK")
	batchB := testBatch(t, d, orgB, entityB, SourceKindBank)

	leaf := testCategoryID(t, d, orgA, leafCategoryCode)

	var txnA, txnB uuid.UUID
	if err := d.InTx(ctx, orgA, func(ctx context.Context, tx pgx.Tx) error {
		out, err := Insert(ctx, tx, orgA, []Transaction{baseTransaction(entityA, accountA, batchA, SourceKindBank)})
		if err != nil {
			return err
		}
		txnA = out[0].ID
		return nil
	}); err != nil {
		t.Fatalf("seeding A's transaction: %v", err)
	}
	if err := d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		out, err := Insert(ctx, tx, orgB, []Transaction{baseTransaction(entityB, accountB, batchB, SourceKindBank)})
		if err != nil {
			return err
		}
		txnB = out[0].ID
		return nil
	}); err != nil {
		t.Fatalf("seeding B's transaction: %v", err)
	}

	// A classifying its own transaction works.
	err := d.InTx(ctx, orgA, func(ctx context.Context, tx pgx.Tx) error {
		_, err := InsertClassification(ctx, tx, orgA, Classification{
			TransactionID: txnA, CategoryID: leaf, EngineLayer: EngineLayerL0,
			Confidence: 0.9, TaxonomyVersion: "v1", RulesetVersion: "v1", EngineVersion: "v1",
			NormalizeVersion: "v1",
		})
		return err
	})
	if err != nil {
		t.Fatalf("A classifying its own transaction: %v", err)
	}

	// A cannot classify B's transaction. Under A's tenant context, a
	// classification row bound to org A has nothing in transactions
	// (org_id=A, id=txnB) to satisfy its own foreign key -- the same
	// non-disclosure every composite foreign key in this schema gives: the
	// refusal is a foreign-key violation, identical to naming a transaction
	// id that was never real.
	err = d.InTx(ctx, orgA, func(ctx context.Context, tx pgx.Tx) error {
		_, err := InsertClassification(ctx, tx, orgA, Classification{
			TransactionID: txnB, CategoryID: leaf, EngineLayer: EngineLayerL0,
			Confidence: 0.9, TaxonomyVersion: "v1", RulesetVersion: "v1", EngineVersion: "v1",
			NormalizeVersion: "v1",
		})
		return err
	})
	if code := pgCode(err); code != "23503" {
		t.Errorf("A classifying B's transaction: SQLSTATE = %q (%v), want 23503", code, err)
	}

	// B reading A's classification does not see it.
	err = d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		_, err := CurrentClassification(ctx, tx, txnA)
		return err
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("B reading A's classification = %v, want pgx.ErrNoRows", err)
	}
}
