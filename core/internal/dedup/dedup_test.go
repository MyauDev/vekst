package dedup

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/ledger"
)

// Task 6.15 (the dedup_skips half): cross-tenant isolation. A cannot read
// B's skips, and the result is indistinguishable from none existing.
func TestSkipsCrossTenantIsolation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	ownerA, ownerB := testUser(t, d), testUser(t, d)
	orgA, entityA := testOrgAndEntity(t, d, ownerA)
	orgB, _ := testOrgAndEntity(t, d, ownerB)
	batchA := testBatch(t, d, orgA, entityA, ledger.SourceKindBank)

	err := d.InTx(ctx, orgA, func(ctx context.Context, tx pgx.Tx) error {
		_, err := InsertSkip(ctx, tx, orgA, Skip{BatchID: batchA, LineNo: 1, Level: LevelD2, DedupHash: uuid.NewString()})
		return err
	})
	if err != nil {
		t.Fatalf("InsertSkip: %v", err)
	}

	err = d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		skips, err := ListSkips(ctx, tx, batchA)
		if err != nil {
			return err
		}
		if len(skips) != 0 {
			t.Errorf("B listing A's skips returned %d rows, want 0", len(skips))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ListSkips as B: %v", err)
	}
}

// Task 6.15 (the internal_transfers half): cross-tenant isolation. A
// cannot read or dismiss B's transfers.
func TestTransfersCrossTenantIsolation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	ownerA, ownerB := testUser(t, d), testUser(t, d)
	orgA, entityA := testOrgAndEntity(t, d, ownerA)
	orgB, _ := testOrgAndEntity(t, d, ownerB)
	accountA1 := testAccount(t, d, orgA, entityA, "NOK")
	accountA2 := testAccount(t, d, orgA, entityA, "NOK")
	batchA := testBatch(t, d, orgA, entityA, ledger.SourceKindBank)

	out := testTransaction(t, d, orgA, entityA, accountA1, batchA, -500)
	in := testTransaction(t, d, orgA, entityA, accountA2, batchA, 500)

	var transferID uuid.UUID
	err := d.InTx(ctx, orgA, func(ctx context.Context, tx pgx.Tx) error {
		row, err := gendb.New(tx).InsertInternalTransferPair(ctx, gendb.InsertInternalTransferPairParams{
			OrgID:   pgtype.UUID{Bytes: orgA.UUID(), Valid: true},
			Column2: pgtype.UUID{Bytes: out.ID, Valid: true},
			Column3: pgtype.UUID{Bytes: in.ID, Valid: true},
		})
		if err != nil {
			return err
		}
		transferID = uuid.UUID(row.ID.Bytes)
		return nil
	})
	if err != nil {
		t.Fatalf("seeding a transfer pair: %v", err)
	}

	err = d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		transfers, err := ListActiveTransfers(ctx, tx, entityA)
		if err != nil {
			return err
		}
		if len(transfers) != 0 {
			t.Errorf("B listing A's transfers returned %d rows, want 0", len(transfers))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ListActiveTransfers as B: %v", err)
	}

	err = d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		_, err := Dismiss(ctx, tx, transferID, ownerB)
		return err
	})
	if !errors.Is(err, ErrTransferNotFound) {
		t.Errorf("B dismissing A's transfer = %v, want ErrTransferNotFound", err)
	}
}

// Task 6.16: fail-closed. Every query added by this change raises 42704
// outside a tenant transaction.
func TestDedupQueriesFailClosedOutsideATenantTransaction(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	someID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	cases := map[string]func(tx pgx.Tx) error{
		"InsertDedupSkip": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).InsertDedupSkip(ctx, gendb.InsertDedupSkipParams{
				OrgID: someID, BatchID: someID, LineNo: 1, Level: LevelD2, DedupHash: "x",
			})
			return err
		},
		"ListDedupSkipsForBatch": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).ListDedupSkipsForBatch(ctx, someID)
			return err
		},
		"CountDedupSkipsForBatch": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).CountDedupSkipsForBatch(ctx, gendb.CountDedupSkipsForBatchParams{BatchID: someID, Level: LevelD2})
			return err
		},
		"FindTransactionByDedupHash": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).FindTransactionByDedupHash(ctx, "x")
			return err
		},
		"InsertInternalTransferPair": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).InsertInternalTransferPair(ctx, gendb.InsertInternalTransferPairParams{
				OrgID: someID, Column2: someID, Column3: pgtype.UUID{Bytes: uuid.New(), Valid: true},
			})
			return err
		},
		"ListActiveInternalTransfers": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).ListActiveInternalTransfers(ctx, someID)
			return err
		},
		"DismissInternalTransfer": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).DismissInternalTransfer(ctx, gendb.DismissInternalTransferParams{ID: someID, DismissedBy: someID})
			return err
		},
		"PairedTransactionIDsForEntity": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).PairedTransactionIDsForEntity(ctx, someID)
			return err
		},
		"CountActiveTransfersForBatch": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).CountActiveTransfersForBatch(ctx, someID)
			return err
		},
		"TransferCandidatesForTransaction": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).TransferCandidatesForTransaction(ctx, gendb.TransferCandidatesForTransactionParams{
				EntityID: someID, Column2: 3, ID: someID, Column4: 100, AccountID: someID, Column6: pgtype.Date{Valid: true},
			})
			return err
		},
	}

	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			err := d.InSystemTx(context.Background(), func(_ context.Context, tx pgx.Tx) error {
				return run(tx)
			})
			if err == nil {
				t.Fatalf("%s succeeded with no tenant context", name)
			}
			if code := pgCode(err); code != "42704" {
				t.Fatalf("%s: SQLSTATE = %q (%v), want 42704", name, code, err)
			}
		})
	}
}

// Task 6.13b: internal_transfer_members' primary key -- not two separate
// unique columns on internal_transfers, which do not see each other -- is
// what refuses a transaction becoming the `out` side of a second pair
// after it is already the `in` side of one.
func TestOnlyOnePairPerTransaction(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	account1 := testAccount(t, d, org, entityID, "NOK")
	account2 := testAccount(t, d, org, entityID, "NOK")
	account3 := testAccount(t, d, org, entityID, "NOK")
	batch := testBatch(t, d, org, entityID, ledger.SourceKindBank)

	out1 := testTransaction(t, d, org, entityID, account1, batch, -500)
	in1 := testTransaction(t, d, org, entityID, account2, batch, 500)
	in2 := testTransaction(t, d, org, entityID, account3, batch, 500)

	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).InsertInternalTransferPair(ctx, gendb.InsertInternalTransferPairParams{
			OrgID:   pgtype.UUID{Bytes: org.UUID(), Valid: true},
			Column2: pgtype.UUID{Bytes: out1.ID, Valid: true},
			Column3: pgtype.UUID{Bytes: in1.ID, Valid: true},
		})
		return err
	})
	if err != nil {
		t.Fatalf("first pair: %v", err)
	}

	// out1 tries to become the out side of a second pair, with in2.
	err = d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).InsertInternalTransferPair(ctx, gendb.InsertInternalTransferPairParams{
			OrgID:   pgtype.UUID{Bytes: org.UUID(), Valid: true},
			Column2: pgtype.UUID{Bytes: out1.ID, Valid: true},
			Column3: pgtype.UUID{Bytes: in2.ID, Valid: true},
		})
		return err
	})
	if code := pgCode(err); code != "23505" {
		t.Errorf("a transaction already in one pair joining a second: SQLSTATE = %q (%v), want 23505", code, err)
	}
}
