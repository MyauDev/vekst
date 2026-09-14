package dedup

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/ledger"
	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/normalize"
)

// testDB, testUser, testOrgAndEntity, testAccount and testBatch mirror
// core/internal/ledger's own harness of the same purpose -- Go does not let
// a _test.go file in one package reuse another's unexported helpers, so
// each package that needs a real tenant to test against grows its own
// small copy.

func testDB(t *testing.T) *db.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping a test that needs a live, migrated Postgres")
	}
	d, err := db.New(context.Background(), db.Config{URL: url})
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}

func testUser(t *testing.T, d *db.DB) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := d.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO users (email) VALUES ($1) RETURNING id`,
			uuid.NewString()+"@dedup.test").Scan(&id)
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

func testOrgAndEntity(t *testing.T, d *db.DB, owner uuid.UUID) (db.OrgID, uuid.UUID) {
	t.Helper()
	org, err := d.CreateOrganization(context.Background(), db.NewOrganization{
		Name:         "Test " + uuid.NewString()[:8],
		Country:      "NO",
		BaseCurrency: "NOK",
		EntityName:   "Test AS",
		CreatorID:    owner,
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	var entityID uuid.UUID
	err = d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		entities, err := gendb.New(tx).ListEntities(ctx)
		if err != nil {
			return err
		}
		entityID = uuid.UUID(entities[0].ID.Bytes)
		return nil
	})
	if err != nil {
		t.Fatalf("reading the organisation's entity: %v", err)
	}
	return org, entityID
}

func testAccount(t *testing.T, d *db.DB, org db.OrgID, entityID uuid.UUID, currency string) uuid.UUID {
	t.Helper()
	var accountID uuid.UUID
	err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		row, err := gendb.New(tx).InsertAccount(ctx, gendb.InsertAccountParams{
			OrgID:    pgtype.UUID{Bytes: org.UUID(), Valid: true},
			EntityID: pgtype.UUID{Bytes: entityID, Valid: true},
			Name:     "Test account " + uuid.NewString()[:8],
			Currency: currency,
		})
		if err != nil {
			return err
		}
		accountID = uuid.UUID(row.ID.Bytes)
		return nil
	})
	if err != nil {
		t.Fatalf("InsertAccount: %v", err)
	}
	return accountID
}

func testBatch(t *testing.T, d *db.DB, org db.OrgID, entityID uuid.UUID, sourceKind string) uuid.UUID {
	t.Helper()
	batchID := uuid.New()
	err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).InsertImportBatch(ctx, gendb.InsertImportBatchParams{
			OrgID:           pgtype.UUID{Bytes: org.UUID(), Valid: true},
			ID:              pgtype.UUID{Bytes: batchID, Valid: true},
			EntityID:        pgtype.UUID{Bytes: entityID, Valid: true},
			SourceKind:      sourceKind,
			UploadedBy:      pgtype.UUID{Bytes: testUser(t, d), Valid: true},
			FileName:        "test.csv",
			DeclaredBytes:   1,
			DeclaredType:    "text/csv",
			FileKey:         "test/" + batchID.String(),
			UploadExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		})
		return err
	})
	if err != nil {
		t.Fatalf("testBatch: InsertImportBatch: %v", err)
	}
	return batchID
}

// testTransaction inserts one transaction directly (bypassing a persist
// job, which this package's own tests have no need to run) and returns it
// with its assigned id.
func testTransaction(t *testing.T, d *db.DB, org db.OrgID, entityID, accountID, batchID uuid.UUID, amountMinor int64) ledger.Transaction {
	t.Helper()
	txn := ledger.Transaction{
		EntityID:         entityID,
		AccountID:        accountID,
		BatchID:          batchID,
		SourceKind:       ledger.SourceKindBank,
		BookedOn:         time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Direction:        ledger.DirectionIncome,
		Amount:           money.Money{CurrencyCode: "NOK", MinorUnits: amountMinor},
		DescriptionNorm:  "test payment",
		NormalizeVersion: normalize.Version,
		DedupHash:        uuid.NewString(),
	}
	if amountMinor < 0 {
		txn.Direction = ledger.DirectionExpense
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

// pgCode returns the SQLSTATE of a Postgres error, or "" if err is not one.
func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	return pgErr.Code
}
