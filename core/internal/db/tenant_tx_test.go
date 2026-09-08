package db

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

// A user to own the organisations these tests create. users is global and
// outside row-level security, so it can be written through InSystemTx.
func testUser(t *testing.T, d *DB) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := d.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO users (email) VALUES ($1) RETURNING id`,
			uuid.NewString()+"@tenancy.test").Scan(&id)
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

// testOrg creates a complete organisation through the real path and returns
// its OrgID -- which is also task 3.11's assertion, since CreateOrganization
// is the only thing here that could have needed a privileged path and does
// not have one.
func testOrg(t *testing.T, d *DB, owner uuid.UUID) OrgID {
	t.Helper()
	org, err := d.CreateOrganization(context.Background(), NewOrganization{
		Name:         "Test " + uuid.NewString()[:8],
		Country:      "NO",
		BaseCurrency: "NOK",
		EntityName:   "Test AS",
		CreatorID:    owner,
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	return org
}

// Task 3.11. An organisation is created under its own policy: the identifier
// is minted first, the transaction is bound to it, and `WITH CHECK (id =
// app_current_org())` passes because the two are the same value. Nothing here
// runs as a privileged role, and no policy is suspended.
func TestCreateOrganizationNeedsNoPrivilegedPath(t *testing.T) {
	d := testDB(t)
	owner := testUser(t, d)
	org := testOrg(t, d, owner)

	if org.IsZero() {
		t.Fatal("CreateOrganization returned a zero OrgID")
	}

	// All three rows are there, and visible only from inside that tenant.
	if err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)
		o, err := q.GetOrganization(ctx)
		if err != nil {
			return err
		}
		if uuid.UUID(o.ID.Bytes) != org.UUID() {
			t.Errorf("GetOrganization returned %s, want %s", uuid.UUID(o.ID.Bytes), org)
		}
		entities, err := q.ListEntities(ctx)
		if err != nil {
			return err
		}
		if len(entities) != 1 {
			t.Errorf("new organisation has %d entities, want 1", len(entities))
		}
		members, err := q.ListMemberships(ctx)
		if err != nil {
			return err
		}
		if len(members) != 1 || members[0].Role != "owner" {
			t.Errorf("new organisation has memberships %+v, want one owner", members)
		}
		return nil
	}); err != nil {
		t.Fatalf("reading back the new organisation: %v", err)
	}
}

// Task 2.3, and the only spec scenario in §2. Row-level security filters
// UPDATE silently: a statement whose target the policy does not admit affects
// zero rows and raises nothing, so without this the caller is told the
// correction was saved when it was not.
func TestUpdateFilteredByPolicyReportsNoRowsAffected(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	orgA := testOrg(t, d, owner)
	orgB := testOrg(t, d, owner)

	// An entity that belongs to B, addressed from a transaction bound to A.
	var bEntity pgtype.UUID
	if err := d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		entities, err := gendb.New(tx).ListEntities(ctx)
		if err != nil {
			return err
		}
		bEntity = entities[0].ID
		return nil
	}); err != nil {
		t.Fatalf("reading org B's entity: %v", err)
	}

	err := d.InTx(ctx, orgA, func(ctx context.Context, tx pgx.Tx) error {
		return ExactlyOneRow(gendb.New(tx).UpdateEntityName(ctx, gendb.UpdateEntityNameParams{
			ID:   bEntity,
			Name: "renamed by another tenant",
		}))
	})
	if !errors.Is(err, ErrNoRowsAffected) {
		t.Fatalf("updating another tenant's row: err = %v, want ErrNoRowsAffected", err)
	}

	// The row is untouched, which is what the error was reporting.
	if err := d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		e, err := gendb.New(tx).GetEntity(ctx, bEntity)
		if err != nil {
			return err
		}
		if e.Name == "renamed by another tenant" {
			t.Error("org A renamed org B's entity")
		}
		return nil
	}); err != nil {
		t.Fatalf("re-reading org B's entity: %v", err)
	}

	// And the same update, from the tenant that owns the row, succeeds -- or
	// the test above would pass against a wrapper that always errored.
	if err := d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		return ExactlyOneRow(gendb.New(tx).UpdateEntityName(ctx, gendb.UpdateEntityNameParams{
			ID:   bEntity,
			Name: "renamed by its owner",
		}))
	}); err != nil {
		t.Fatalf("org B updating its own entity: %v", err)
	}
}

// Task 3.7. The zero value is rejected before a transaction is opened, and the
// error names the constructor to use rather than leaving the caller to work
// out why a query returned nothing.
func TestInTxRejectsAZeroOrgID(t *testing.T) {
	d := testDB(t)

	called := false
	err := d.InTx(context.Background(), OrgID{}, func(context.Context, pgx.Tx) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("InTx accepted a zero OrgID")
	}
	if called {
		t.Error("InTx ran the callback with a zero OrgID")
	}
	for _, want := range []string{"OrgIDForSession", "OrgIDFromJobArgs", "OrgIDForNewOrg"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s; a caller has to guess the fix", err, want)
		}
	}
}

// pgCode returns the SQLSTATE of a Postgres error, or "" if err is not one.
// Asserting on the code rather than the message keeps these tests from
// breaking on a Postgres wording change.
func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	return pgErr.Code
}

// Task 3.9. The leak the transaction-local flag exists to prevent. A pool of
// exactly one connection guarantees the second transaction lands on the same
// backend as the first, which is the only way to observe this.
func TestTenantContextDoesNotOutliveItsTransaction(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping a test that needs a live, migrated Postgres")
	}
	// One connection, so "the same pooled connection" is not a hope.
	d, err := New(context.Background(), Config{URL: url, MaxConns: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer d.Close()
	ctx := context.Background()

	owner := testUser(t, d)
	org := testOrg(t, d, owner)

	// First: a real tenant transaction that commits.
	if err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var n int
		return tx.QueryRow(ctx, `SELECT count(*) FROM entities`).Scan(&n)
	}); err != nil {
		t.Fatalf("first transaction: %v", err)
	}

	// Second: no tenant context at all, on that same connection. It must
	// raise -- not return org A's rows, and not return zero rows either.
	var n int
	err = d.InSystemTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM entities`).Scan(&n)
	})
	if err == nil {
		t.Fatalf("an untenanted transaction on a reused connection returned %d row(s); "+
			"the previous transaction's tenant context outlived it", n)
	}
	if code := pgCode(err); code != "42704" {
		t.Fatalf("untenanted read after a tenant transaction: SQLSTATE %s (%v), want 42704", code, err)
	}
}

// Task 3.10. Rollback must leave nothing behind either -- the transaction-local
// setting is reverted whether the transaction commits or aborts.
func TestRolledBackTenantTransactionLeavesNoContext(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping a test that needs a live, migrated Postgres")
	}
	d, err := New(context.Background(), Config{URL: url, MaxConns: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer d.Close()
	ctx := context.Background()

	owner := testUser(t, d)
	org := testOrg(t, d, owner)

	if err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM entities`).Scan(&n); err != nil {
			return err
		}
		return errBoom
	}); !errors.Is(err, errBoom) {
		t.Fatalf("InTx error = %v, want errBoom", err)
	}

	var n int
	err = d.InSystemTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM entities`).Scan(&n)
	})
	if err == nil {
		t.Fatalf("after a rolled-back tenant transaction, an untenanted read returned %d row(s)", n)
	}
	if code := pgCode(err); code != "42704" {
		t.Fatalf("untenanted read after a rollback: SQLSTATE %s (%v), want 42704", code, err)
	}
}

// Task 3.6. A caller may only act for an organisation they belong to, and the
// refusal must not distinguish "you are not a member" from "no such
// organisation" -- otherwise this call becomes the membership oracle the
// schema spends composite keys closing.
func TestOrgIDForSessionRefusesANonMemberIndistinguishably(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	member := testUser(t, d)
	stranger := testUser(t, d)
	org := testOrg(t, d, member)

	// The member gets in, with their role.
	got, role, err := d.OrgIDForSession(ctx, member, org.UUID())
	if err != nil {
		t.Fatalf("OrgIDForSession for a member: %v", err)
	}
	if got.UUID() != org.UUID() {
		t.Errorf("OrgIDForSession returned %s, want %s", got, org)
	}
	if role != "owner" {
		t.Errorf("role = %q, want owner", role)
	}

	// A stranger, for an organisation that exists.
	_, _, errStranger := d.OrgIDForSession(ctx, stranger, org.UUID())
	if errStranger == nil {
		t.Fatal("OrgIDForSession admitted a user with no membership")
	}

	// The same user, for an organisation that exists nowhere.
	_, _, errNoSuchOrg := d.OrgIDForSession(ctx, stranger, uuid.New())
	if errNoSuchOrg == nil {
		t.Fatal("OrgIDForSession admitted a nonexistent organisation")
	}

	if errStranger.Error() != errNoSuchOrg.Error() {
		t.Errorf("the two refusals differ, which makes this a membership oracle:\n"+
			"  not a member:  %v\n  no such org:   %v", errStranger, errNoSuchOrg)
	}
	if !errors.Is(errStranger, ErrNotAMember) || !errors.Is(errNoSuchOrg, ErrNotAMember) {
		t.Errorf("want both refusals to be ErrNotAMember; got %v and %v", errStranger, errNoSuchOrg)
	}
}

// OrgIDFromJobArgs is the worker door, and a job that carries no organisation
// must fail rather than run with no tenant context (task 4.3's assertion, made
// here because it is a property of the constructor rather than of a worker).
func TestOrgIDFromJobArgsRejectsAnAbsentOrganisation(t *testing.T) {
	if _, err := OrgIDFromJobArgs(TenantJobArgs{}); err == nil {
		t.Fatal("OrgIDFromJobArgs accepted job arguments with no organisation")
	}
	id := uuid.New()
	org, err := OrgIDFromJobArgs(TenantJobArgs{OrgID: id})
	if err != nil {
		t.Fatalf("OrgIDFromJobArgs with an organisation: %v", err)
	}
	if org.UUID() != id {
		t.Errorf("OrgIDFromJobArgs returned %s, want %s", org, id)
	}
}
