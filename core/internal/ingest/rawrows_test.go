package ingest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MyauDev/vekst/core/internal/money"
)

func sampleRows() []Row {
	debit, _ := money.New("BYN", 0)
	credit, _ := money.New("BYN", 15000)
	return []Row{
		{LineNo: 12, BookedOn: "01.09.2026", Description: "coffee", Debit: debit, Credit: credit},
		{LineNo: 13, BookedOn: "02.09.2026", Description: "rent", Debit: credit, Credit: debit},
	}
}

func TestPersistRawRowsRoundTrips(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	batch, _, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "x.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}

	rows := sampleRows()
	var affected int64
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		affected, err = PersistRawRows(ctx, tx, org, batch.ID, rows)
		return err
	})
	if err != nil {
		t.Fatalf("PersistRawRows: %v", err)
	}
	if affected != int64(len(rows)) {
		t.Errorf("affected = %d, want %d", affected, len(rows))
	}

	var count int
	var lineNos []int32
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM raw_rows WHERE batch_id = $1`,
			pgtype.UUID{Bytes: batch.ID, Valid: true},
		).Scan(&count); err != nil {
			return err
		}
		r, err := tx.Query(ctx,
			`SELECT line_no FROM raw_rows WHERE batch_id = $1 ORDER BY line_no`,
			pgtype.UUID{Bytes: batch.ID, Valid: true})
		if err != nil {
			return err
		}
		defer r.Close()
		for r.Next() {
			var n int32
			if err := r.Scan(&n); err != nil {
				return err
			}
			lineNos = append(lineNos, n)
		}
		return r.Err()
	})
	if err != nil {
		t.Fatalf("reading back raw_rows: %v", err)
	}
	if count != len(rows) {
		t.Errorf("count = %d, want %d", count, len(rows))
	}
	if len(lineNos) != 2 || lineNos[0] != 12 || lineNos[1] != 13 {
		t.Errorf("line_no values = %v, want [12 13]", lineNos)
	}
}

// Task 4.2.
func TestRawRowsCrossTenantIsolation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	ownerA := testUser(t, env.db)
	ownerB := testUser(t, env.db)
	orgA, entityA := testOrgAndEntity(t, env.db, ownerA)
	orgB, _ := testOrgAndEntity(t, env.db, ownerB)

	batchA, _, err := env.svc.CreateImportBatch(ctx, ownerA, orgA.UUID(), CreateBatchInput{
		EntityID: entityA, SourceKind: SourceKindBank, FileName: "a.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	if err := env.db.InTx(ctx, orgA, func(ctx context.Context, tx pgx.Tx) error {
		_, err := PersistRawRows(ctx, tx, orgA, batchA.ID, sampleRows())
		return err
	}); err != nil {
		t.Fatalf("PersistRawRows for A: %v", err)
	}

	// B, reading its own tenant context, never sees A's rows -- not by
	// batch id, and not at all.
	var countByBatchID, countAll int
	err = env.db.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM raw_rows WHERE batch_id = $1`,
			pgtype.UUID{Bytes: batchA.ID, Valid: true}).Scan(&countByBatchID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM raw_rows`).Scan(&countAll)
	})
	if err != nil {
		t.Fatalf("B reading raw_rows: %v", err)
	}
	if countByBatchID != 0 || countAll != 0 {
		t.Errorf("B saw %d row(s) by A's batch id and %d total, want 0 and 0", countByBatchID, countAll)
	}

	// B cannot even write against A's batch: the composite foreign key
	// (org_id, batch_id) has no row for (B, A's batch id) to match, whether
	// or not B knows that id.
	err = env.db.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		_, err := PersistRawRows(ctx, tx, orgB, batchA.ID, sampleRows())
		return err
	})
	if err == nil {
		t.Fatal("B inserted raw_rows against A's batch id")
	}
	if code := pgCode(err); code != "23503" {
		t.Errorf("SQLSTATE = %q (%v), want 23503 (foreign_key_violation)", code, err)
	}
}

func TestRawRowsFailClosedOutsideATenantTransaction(t *testing.T) {
	env := newTestEnv(t)
	someBatch := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	err := env.db.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var n int
		return tx.QueryRow(ctx, `SELECT count(*) FROM raw_rows WHERE batch_id = $1`, someBatch).Scan(&n)
	})
	if err == nil {
		t.Fatal("reading raw_rows succeeded with no tenant context")
	}
	if code := pgCode(err); code != "42704" {
		t.Fatalf("SQLSTATE = %q (%v), want 42704", code, err)
	}
}

// One row per line per batch: a retried persist must not double the table.
func TestRawRowsRejectsADuplicateLineNumber(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	batch, _, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "x.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}

	rows := sampleRows()
	if err := env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := PersistRawRows(ctx, tx, org, batch.ID, rows)
		return err
	}); err != nil {
		t.Fatalf("first PersistRawRows: %v", err)
	}

	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := PersistRawRows(ctx, tx, org, batch.ID, rows)
		return err
	})
	if err == nil {
		t.Fatal("a second persist of the same lines succeeded")
	}
	if code := pgCode(err); code != "23505" {
		t.Errorf("SQLSTATE = %q (%v), want 23505 (unique_violation)", code, err)
	}
}
