package ledger

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
	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/normalize"
)

// Two of migration 005's own seeded 'v1' categories, picked for what they
// are rather than created by a test: '0401010201' ("1С Cloud") is a leaf
// that is not computed, so a classification may target it; '91' ("GM") is
// both a section (is_leaf false) and a computed line (is_computed true),
// so it is what rules_category_is_visible refuses.
const (
	leafCategoryCode              = "0401010201"
	sectionOrComputedCategoryCode = "91"
)

// testDB, testUser and testOrgAndEntity mirror core/internal/ingest's own
// test harness of the same purpose -- Go does not let a _test.go file in one
// package reuse another's unexported helpers, so each package that needs a
// real tenant to test against grows its own small copy.

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
			uuid.NewString()+"@ledger.test").Scan(&id)
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

// testOrgAndEntity creates a real organisation -- base currency NOK -- and
// returns its own OrgID plus the one entity CreateOrganization gives every
// organisation in the Demo.
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

// testAccount creates one account in currency, for entityID.
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

// testBatch creates one import_batches row in awaiting_upload, satisfying
// every column migration 008 made NOT NULL -- with dummy values, since
// nothing here exercises the upload state machine, only transactions'
// foreign key onto its batch and the trigger holding source_kind equal.
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

// baseTransaction returns a Transaction that satisfies every NOT NULL
// column and every CHECK by construction -- account, entity and batch all
// belong to org, and the batch's source_kind matches. Tests mutate the
// fields they care about from here.
func baseTransaction(entityID, accountID, batchID uuid.UUID, sourceKind string) Transaction {
	return Transaction{
		EntityID:         entityID,
		AccountID:        accountID,
		BatchID:          batchID,
		SourceKind:       sourceKind,
		PostingNo:        0,
		BookedOn:         time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Direction:        DirectionExpense,
		Amount:           money.Money{CurrencyCode: "NOK", MinorUnits: 12345},
		CounterpartyRaw:  "Test Counterparty AS",
		CounterpartyKey:  "name:test-counterparty-as",
		DescriptionRaw:   "Test payment",
		DescriptionNorm:  "test payment",
		NormalizeVersion: normalize.Version,
		DedupHash:        uuid.NewString(),
	}
}

// testCategoryID looks up one of the shared 'v1' taxonomy's own categories
// by its natural key -- these are seeded by migration 005 and exist in
// every organisation, never created by a test.
func testCategoryID(t *testing.T, d *db.DB, org db.OrgID, code string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		row, err := gendb.New(tx).CategoryByCode(ctx, gendb.CategoryByCodeParams{
			TaxonomyVersion: "v1",
			OrgID:           pgtype.UUID{Valid: false},
			Code:            code,
		})
		if err != nil {
			return err
		}
		id = uuid.UUID(row.ID.Bytes)
		return nil
	})
	if err != nil {
		t.Fatalf("CategoryByCode(%q): %v", code, err)
	}
	return id
}

// pgCode returns the SQLSTATE of a Postgres error, or "" if err is not one --
// the same helper core/internal/db and core/internal/ingest's own tests use,
// so a test asserts on the code rather than on wording that may change.
func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	return pgErr.Code
}
