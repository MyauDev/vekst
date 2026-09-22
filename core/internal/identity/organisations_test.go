package identity

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MyauDev/vekst/core/internal/db"
)

func organisationsTestDB(t *testing.T) *db.DB {
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

func organisationsSeedUser(t *testing.T, d *db.DB) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := d.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`,
			uuid.NewString()+"@organisations.test").Scan(&id)
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

// Task 7.2. Cross-tenant isolation, GetCurrentUser: a user who belongs to A
// only receives A, with B absent from the response, while both organisations
// exist.
func TestOrganisationsIsolatesOtherOrganisations(t *testing.T) {
	d := organisationsTestDB(t)
	svc := &Service{database: d}
	ctx := context.Background()

	userA := organisationsSeedUser(t, d)
	orgA, err := d.CreateOrganization(ctx, db.NewOrganization{
		Name: "Org A", Country: "BY", BaseCurrency: "BYN", EntityName: "A Entity", CreatorID: userA,
	})
	if err != nil {
		t.Fatalf("creating org A: %v", err)
	}

	// A second organisation exists, owned by somebody else entirely -- not
	// merely a second caller, so that "absent from the response" cannot be
	// explained by userA never having had access in the first place.
	userB := organisationsSeedUser(t, d)
	if _, err := d.CreateOrganization(ctx, db.NewOrganization{
		Name: "Org B", Country: "KZ", BaseCurrency: "KZT", EntityName: "B Entity", CreatorID: userB,
	}); err != nil {
		t.Fatalf("creating org B: %v", err)
	}

	orgs, err := svc.Organisations(ctx, userA)
	if err != nil {
		t.Fatalf("Organisations(userA): %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("userA belongs to %d organisation(s), want 1", len(orgs))
	}
	if orgs[0].ID != orgA.UUID() {
		t.Errorf("Organisations(userA) returned %s, want org A (%s)", orgs[0].ID, orgA.UUID())
	}
	if orgs[0].Role != "owner" {
		t.Errorf("Organisations(userA)[0].Role = %q, want owner", orgs[0].Role)
	}
	if len(orgs[0].Entities) != 1 || orgs[0].Entities[0].Name != "A Entity" {
		t.Errorf("Organisations(userA)[0].Entities = %+v, want one entity named A Entity", orgs[0].Entities)
	}
}

// A first-time caller with no membership at all receives an empty list, not
// an error -- the first-run signal, and a fact rather than a failure.
func TestOrganisationsIsEmptyForAFirstTimeCaller(t *testing.T) {
	d := organisationsTestDB(t)
	svc := &Service{database: d}

	user := organisationsSeedUser(t, d)
	orgs, err := svc.Organisations(context.Background(), user)
	if err != nil {
		t.Fatalf("Organisations: %v", err)
	}
	if len(orgs) != 0 {
		t.Fatalf("Organisations for a first-time caller returned %d organisation(s), want 0", len(orgs))
	}
}
