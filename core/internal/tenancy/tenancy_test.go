package tenancy_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/tenancy"
)

// These run against a live, migrated Postgres reached as vekst_app, the same
// arrangement core/internal/db's and core/internal/review's own tests use.

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

func seedUser(t *testing.T, d *db.DB) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := d.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`,
			uuid.NewString()+"@tenancy.test").Scan(&id)
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

func codeOf(err error) string {
	var coded *tenancy.Err
	if errors.As(err, &coded) {
		return coded.Code
	}
	return ""
}

var validNew = tenancy.New{
	Name:         "Test Org",
	Country:      "BY",
	BaseCurrency: "BYN",
	EntityName:   "Test Entity",
}

// Task 7.5. A second CreateOrganization from the same caller is refused with
// already_a_member and writes nothing.
func TestSecondOrganizationIsRefused(t *testing.T) {
	d := testDB(t)
	svc := tenancy.NewService(d)
	ctx := context.Background()
	user := seedUser(t, d)

	first, err := svc.Create(ctx, user, validNew)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if first.OrgID == uuid.Nil {
		t.Fatal("first Create returned a zero organisation id")
	}

	before, err := d.MembershipsForUser(ctx, user)
	if err != nil {
		t.Fatalf("MembershipsForUser before second Create: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("caller has %d membership(s) after one Create, want 1", len(before))
	}

	_, err = svc.Create(ctx, user, tenancy.New{
		Name: "Second Org", Country: "KZ", BaseCurrency: "KZT", EntityName: "Second Entity",
	})
	if codeOf(err) != tenancy.CodeAlreadyMember {
		t.Fatalf("second Create: err = %v, want code %s", err, tenancy.CodeAlreadyMember)
	}

	after, err := d.MembershipsForUser(ctx, user)
	if err != nil {
		t.Fatalf("MembershipsForUser after refused second Create: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("caller has %d membership(s) after a refused second Create, want 1 (nothing written)", len(after))
	}
	if after[0].Org.UUID() != first.OrgID {
		t.Error("the surviving membership is not the one the first Create wrote")
	}
}

// Task 7.6. An unsupported currency is refused before any row is written.
// XXX is used deliberately: it passes organizations' own CHECK
// (base_currency ~ '^[A-Z]{3}$'), so this test fails if the Go validation
// against core/internal/money's exponent map is ever dropped in favour of
// relying on the database alone.
func TestUnsupportedCurrencyIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	d := testDB(t)
	svc := tenancy.NewService(d)
	ctx := context.Background()
	user := seedUser(t, d)

	_, err := svc.Create(ctx, user, tenancy.New{
		Name: "Bad Currency Org", Country: "BY", BaseCurrency: "XXX", EntityName: "Entity",
	})
	if codeOf(err) != tenancy.CodeUnsupportedCurrency {
		t.Fatalf("Create with XXX: err = %v, want code %s", err, tenancy.CodeUnsupportedCurrency)
	}

	memberships, err := d.MembershipsForUser(ctx, user)
	if err != nil {
		t.Fatalf("MembershipsForUser: %v", err)
	}
	if len(memberships) != 0 {
		t.Fatalf("caller has %d membership(s) after a refused Create, want 0 (nothing written)", len(memberships))
	}
}

// An unsupported country is refused the same way, before any row is written.
func TestUnsupportedCountryIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	d := testDB(t)
	svc := tenancy.NewService(d)
	ctx := context.Background()
	user := seedUser(t, d)

	_, err := svc.Create(ctx, user, tenancy.New{
		Name: "Bad Country Org", Country: "US", BaseCurrency: "USD", EntityName: "Entity",
	})
	if codeOf(err) != tenancy.CodeUnsupportedCountry {
		t.Fatalf("Create with US: err = %v, want code %s", err, tenancy.CodeUnsupportedCountry)
	}

	memberships, err := d.MembershipsForUser(ctx, user)
	if err != nil {
		t.Fatalf("MembershipsForUser: %v", err)
	}
	if len(memberships) != 0 {
		t.Fatalf("caller has %d membership(s) after a refused Create, want 0 (nothing written)", len(memberships))
	}
}
