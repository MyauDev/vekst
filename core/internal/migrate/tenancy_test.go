package migrate

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// The proof that migration 00004's policies isolate, rather than merely exist.
//
// Every test here runs as vekst_app -- the role core actually connects as --
// against a database migrated by a non-superuser vekst_migrator, because that
// is the only shape in which row-level security means anything (design D0). A
// superuser owner bypasses RLS unconditionally, FORCE included, so the same
// assertions against a superuser-owned schema would pass while proving nothing.
//
// Tenant context is set with set_config(..., true) inside an explicit
// transaction throughout, never as a session variable: that is what db.InTx
// will do, and a session-level SET would outlive the transaction on a pooled
// connection -- the exact leak task 3.9 exists to catch.

const (
	orgA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	orgB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	// A UUID belonging to no organisation, for the comparison in task 1.7:
	// a reference to another tenant's row must fail exactly as a reference to
	// nothing at all does.
	orgNobody = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

// tenantFixture is a migrated database holding two complete organisations,
// plus the identifiers needed to address each one's rows.
type tenantFixture struct {
	migrator *sql.DB // vekst_migrator: seeds, and reads back past the policy
	app      *sql.DB // vekst_app: everything under test
	entityA  string
	entityB  string
	userA    string
}

func newTenantFixture(t *testing.T, dbName string) *tenantFixture {
	t.Helper()
	migratorURL := testMigratorURL(t, dbName)
	if err := Up(context.Background(), migratorURL); err != nil {
		t.Fatalf("Up: %v", err)
	}

	migrator, err := sql.Open("pgx", migratorURL)
	if err != nil {
		t.Fatalf("opening migrator connection: %v", err)
	}
	t.Cleanup(func() { migrator.Close() })

	app, err := sql.Open("pgx", appURL(t, migratorURL))
	if err != nil {
		t.Fatalf("opening app connection: %v", err)
	}
	t.Cleanup(func() { app.Close() })

	f := &tenantFixture{migrator: migrator, app: app}

	// users is global and has no policy (00003, allowlisted): a person exists
	// before any organisation does.
	if err := migrator.QueryRow(
		`INSERT INTO users (email) VALUES ('a@example.test') RETURNING id`).Scan(&f.userA); err != nil {
		t.Fatalf("seeding user: %v", err)
	}

	// Seeding goes through the policy rather than around it. That is design
	// D4: an organisation is created by naming its own identifier as the
	// tenant context, so WITH CHECK (id = app_current_org()) passes and no
	// privileged path is needed to create a tenant. If this stops working,
	// organisation creation is broken, and finding that out in the fixture is
	// as good a place as any.
	f.entityA = seedOrg(t, migrator, orgA, "Org A", f.userA)
	f.entityB = seedOrg(t, migrator, orgB, "Org B", "")

	return f
}

// seedOrg creates one organisation with an entity and an account, all inside a
// single transaction bound to that organisation, and returns the entity's id.
func seedOrg(t *testing.T, db *sql.DB, orgID, name, userID string) string {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck // committed below; this is the failure path

	if _, err := tx.Exec(`SELECT set_config('app.org_id', $1, true)`, orgID); err != nil {
		t.Fatalf("setting tenant context: %v", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO organizations (id, name, country, base_currency) VALUES ($1, $2, 'NO', 'NOK')`,
		orgID, name); err != nil {
		t.Fatalf("seeding organisation %s: %v", name, err)
	}
	var entityID string
	if err := tx.QueryRow(
		`INSERT INTO entities (org_id, name) VALUES ($1, $2) RETURNING id`,
		orgID, name+" AS").Scan(&entityID); err != nil {
		t.Fatalf("seeding entity for %s: %v", name, err)
	}
	// Both organisations use the same external_ref on purpose -- task 1.8's
	// premise is that this is not a collision.
	if _, err := tx.Exec(
		`INSERT INTO accounts (org_id, entity_id, name, currency, external_ref)
		 VALUES ($1, $2, 'Operating', 'NOK', 'ACME-4471')`,
		orgID, entityID); err != nil {
		t.Fatalf("seeding account for %s: %v", name, err)
	}
	if userID != "" {
		if _, err := tx.Exec(
			`INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, 'owner')`,
			orgID, userID); err != nil {
			t.Fatalf("seeding membership for %s: %v", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seeding %s: %v", name, err)
	}
	return entityID
}

// inTenantTx runs fn inside a transaction bound to orgID, mirroring what
// db.InTx will do -- including its commit-on-success contract, so that a write
// a test makes is actually there for the next one to collide with. It returns
// fn's error unwrapped so a test can inspect the SQLSTATE.
func inTenantTx(t *testing.T, db *sql.DB, orgID string, fn func(*sql.Tx) error) error {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	if _, err := tx.Exec(`SELECT set_config('app.org_id', $1, true)`, orgID); err != nil {
		tx.Rollback() //nolint:errcheck // already failing
		t.Fatalf("setting tenant context: %v", err)
	}
	if err := fn(tx); err != nil {
		tx.Rollback() //nolint:errcheck // returning fn's error, which is what the test asserts on
		return err
	}
	return tx.Commit()
}

// sqlState returns the five-character SQLSTATE of a Postgres error, or "" if
// err is not one. Asserting on the code rather than on the message is what
// keeps these tests from breaking on a Postgres wording change.
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	return pgErr.Code
}

// Task 1.3. current_setting is called with one argument so that an unset
// app.org_id raises rather than returning NULL. With the two-argument form
// every policy would evaluate to false and a query outside a tenant
// transaction would return zero rows with no error -- which a report renders
// as "this customer has no revenue".
func TestTenantContextAccessorRaisesWhenUnset(t *testing.T) {
	f := newTenantFixture(t, "vekst_tenancy_accessor_test")

	var got string
	err := f.app.QueryRow(`SELECT app_current_org()::text`).Scan(&got)
	if err == nil {
		t.Fatalf("app_current_org() with no context returned %q; want an error", got)
	}
	if code := sqlState(err); code != "42704" {
		t.Fatalf("app_current_org() with no context: SQLSTATE %s (%v), want 42704 undefined_object", code, err)
	}
}

// Task 1.5. The policies filter reads, and the queries carry no organisation
// predicate of their own -- that is the whole point: isolation that does not
// depend on every future query remembering a WHERE clause.
func TestCrossTenantRead(t *testing.T) {
	f := newTenantFixture(t, "vekst_tenancy_read_test")

	for _, table := range []string{"organizations", "entities", "accounts", "memberships"} {
		err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			// One row each for org A: the fixture seeds an organisation, an
			// entity, an account and a membership per tenant (B has no
			// membership, which is why B's absence below is checked by
			// tenant column rather than by count).
			var n int
			if err := tx.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				t.Errorf("%s visible to org A = %d row(s), want 1", table, n)
			}
			// And nothing of B's is reachable by name.
			var leaked int
			if err := tx.QueryRow(
				`SELECT count(*) FROM `+table+` WHERE `+tenantColumn(table)+` = $1`,
				orgB).Scan(&leaked); err != nil {
				return err
			}
			if leaked != 0 {
				t.Errorf("%s: org A can see %d of org B's rows", table, leaked)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("reading %s as org A: %v", table, err)
		}
	}
}

// tenantColumn names the column each table's policy restricts. organizations
// is the exception the coverage test (D6) also has to know about: the
// organisation is the tenant, so its own primary key is the tenant key.
func tenantColumn(table string) string {
	if table == "organizations" {
		return "id"
	}
	return "org_id"
}

// Task 1.6. WITH CHECK is what stops a tenant writing into another tenant,
// on insert and on update alike. An UPDATE that would move a row across the
// boundary is the quieter of the two and the one worth naming.
func TestCrossTenantWrite(t *testing.T) {
	f := newTenantFixture(t, "vekst_tenancy_write_test")

	// Insert stamped with B's identifier, from a transaction bound to A.
	err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO entities (org_id, name) VALUES ($1, 'smuggled')`, orgB)
		return err
	})
	if err == nil {
		t.Fatal("org A inserted an entity stamped org B; WITH CHECK is not doing its job")
	}
	if code := sqlState(err); code != "42501" {
		t.Fatalf("cross-tenant insert: SQLSTATE %s (%v), want 42501", code, err)
	}

	// Update moving one of A's own rows to B.
	err = inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE entities SET org_id = $1`, orgB)
		return err
	})
	if err == nil {
		t.Fatal("org A moved an entity to org B; WITH CHECK is not applied to UPDATE")
	}
	if code := sqlState(err); code != "42501" {
		t.Fatalf("cross-tenant update: SQLSTATE %s (%v), want 42501", code, err)
	}

	// B's rows are untouched.
	var name string
	if err := inTenantTx(t, f.app, orgB, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT name FROM entities`).Scan(&name)
	}); err != nil {
		t.Fatalf("re-reading org B's entity: %v", err)
	}
	if name != "Org B AS" {
		t.Fatalf("org B's entity is now %q; it was modified across the tenant boundary", name)
	}
}

// Task 1.7. Referential-integrity checks are not subject to row-level
// security, which is why the foreign key has to carry org_id itself (design
// D1). Without that, referencing another tenant's entity would succeed where
// referencing nothing failed -- and the difference between those two outcomes
// is the disclosure.
func TestCrossTenantForeignKeyRevealsNothing(t *testing.T) {
	f := newTenantFixture(t, "vekst_tenancy_fk_test")

	insertAccount := func(entityID string) error {
		return inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			_, err := tx.Exec(
				`INSERT INTO accounts (org_id, entity_id, name, currency)
				 VALUES ($1, $2, 'probe', 'NOK')`, orgA, entityID)
			return err
		})
	}

	// The probe that matters: an entity that exists, owned by another tenant.
	errOther := insertAccount(f.entityB)
	if errOther == nil {
		t.Fatal("org A created an account against org B's entity; the composite foreign key is not carrying org_id")
	}
	// The control: an entity that exists nowhere.
	errNobody := insertAccount(orgNobody)
	if errNobody == nil {
		t.Fatal("an account referencing a nonexistent entity was accepted")
	}

	// Both must fail the same way, or the error itself answers "does this
	// identifier belong to somebody else?"
	if a, b := sqlState(errOther), sqlState(errNobody); a != b {
		t.Fatalf("cross-tenant reference gives SQLSTATE %s and a nonexistent one gives %s; the difference discloses another tenant's row", a, b)
	}
	if code := sqlState(errOther); code != "23503" {
		t.Fatalf("cross-tenant reference: SQLSTATE %s (%v), want 23503 foreign_key_violation", code, errOther)
	}
	if errOther.Error() != errNobody.Error() {
		t.Errorf("the two failures differ in text:\n  other tenant: %v\n  nonexistent:  %v", errOther, errNobody)
	}
}

// Task 1.8. A uniqueness constraint is a louder oracle than a foreign key: it
// discloses a value rather than an identifier. Two organisations holding the
// same external_ref must not collide.
func TestUniquenessIsScopedByOrganisation(t *testing.T) {
	f := newTenantFixture(t, "vekst_tenancy_unique_test")

	// The fixture already gave both organisations an account with
	// external_ref 'ACME-4471'. That it seeded at all is half the proof; this
	// is the other half, from the application role, after the fact.
	err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO accounts (org_id, entity_id, name, currency, external_ref)
			 VALUES ($1, $2, 'second', 'NOK', 'ACME-9999')`, orgA, f.entityA)
		return err
	})
	if err != nil {
		t.Fatalf("org A adding a distinct external_ref: %v", err)
	}

	// And a genuine within-tenant duplicate still collides, or the constraint
	// is scoped so widely it constrains nothing.
	err = inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO accounts (org_id, entity_id, name, currency, external_ref)
			 VALUES ($1, $2, 'duplicate', 'NOK', 'ACME-9999')`, orgA, f.entityA)
		return err
	})
	if err == nil {
		t.Fatal("a duplicate external_ref within one organisation was accepted; the unique constraint is not doing its job")
	}
	if code := sqlState(err); code != "23505" {
		t.Fatalf("within-tenant duplicate: SQLSTATE %s (%v), want 23505 unique_violation", code, err)
	}
}

// Task 1.9. The failure mode this whole design is built against: not a leak,
// but a silence. A query with no tenant context must raise, because zero rows
// is indistinguishable from a customer who genuinely has no data -- and a
// report renders it as such.
//
// The raise does not depend on the table holding rows. app_current_org() takes
// no arguments and is STABLE, so the planner folds it to a constant while
// estimating selectivity; the error arrives before any tuple is read. Both
// cases are asserted below, because a per-row qual would have passed the first
// and failed the second, and the difference would have gone unnoticed until a
// new tenant's first, empty report.
func TestNoTenantContextRaisesRatherThanReturningNothing(t *testing.T) {
	f := newTenantFixture(t, "vekst_tenancy_noctx_test")

	for _, table := range []string{"organizations", "entities", "accounts", "memberships"} {
		var n int
		err := f.app.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n)
		if err == nil {
			t.Errorf("%s with no tenant context returned %d row(s) instead of raising", table, n)
			continue
		}
		if code := sqlState(err); code != "42704" {
			t.Errorf("%s with no tenant context: SQLSTATE %s (%v), want 42704 undefined_object", table, code, err)
		}
	}

	// The second way to have no tenant context, and the one that actually
	// happens in production. set_config(..., true) is reverted when the
	// transaction ends, but reverting does not unregister the parameter -- it
	// is left holding the empty string, and core runs on a connection pool, so
	// every reuse of a connection that has served a tenant transaction takes
	// this path. A bare ''::uuid would raise 22P02 here rather than 42704;
	// app_current_org() normalises it, so one condition has one code.
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		var n int
		return tx.QueryRow(`SELECT count(*) FROM entities`).Scan(&n)
	}); err != nil {
		t.Fatalf("priming the connection with a tenant transaction: %v", err)
	}
	for _, table := range []string{"organizations", "entities", "accounts", "memberships"} {
		var n int
		err := f.app.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n)
		if err == nil {
			t.Errorf("%s after a tenant transaction ended returned %d row(s) instead of raising", table, n)
			continue
		}
		if code := sqlState(err); code != "42704" {
			t.Errorf("%s on a connection whose tenant transaction has ended: SQLSTATE %s (%v), want 42704 -- "+
				"the same code as a connection that never had a context, or callers must know two", table, code, err)
		}
	}

	// The empty-table case, explicitly. accounts holds rows for both
	// organisations above, so this needs a table that is genuinely empty:
	// delete A's and B's rows first, as their own tenants, then ask again.
	for _, org := range []string{orgA, orgB} {
		if err := inTenantTx(t, f.app, org, func(tx *sql.Tx) error {
			_, err := tx.Exec(`DELETE FROM accounts`)
			return err
		}); err != nil {
			t.Fatalf("emptying accounts for %s: %v", org, err)
		}
	}
	var n int
	err := f.app.QueryRow(`SELECT count(*) FROM accounts`).Scan(&n)
	if err == nil {
		t.Fatalf("an empty accounts table with no tenant context returned %d rather than raising; "+
			"fail-closed is holding only where rows exist", n)
	}
	if code := sqlState(err); code != "42704" {
		t.Fatalf("empty table with no tenant context: SQLSTATE %s (%v), want 42704", code, err)
	}
}

// Task 1.8, the catalog half. A uniqueness constraint is not subject to
// row-level security, so one that does not carry org_id is a cross-tenant
// oracle no policy can close: under context A, an insert colliding with
// organisation B's row raises unique_violation where an unused value succeeds,
// and the difference is the disclosure.
//
// This enumerates rather than spot-checks, because the constraint that matters
// is the one a future migration adds without thinking about it. organizations
// is the one exception, and for the same reason it is the exception in the
// coverage test: the organisation is the tenant, so its own primary key is the
// tenant key.
func TestEveryUniquenessConstraintOnATenantTableLeadsWithOrgID(t *testing.T) {
	f := newTenantFixture(t, "vekst_tenancy_constraints_test")

	rows, err := f.migrator.Query(`
		SELECT c.relname, con.conname,
		       (SELECT a.attname
		          FROM unnest(con.conkey) WITH ORDINALITY AS k(attnum, ord)
		          JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
		         WHERE k.ord = 1) AS leading_column
		  FROM pg_constraint con
		  JOIN pg_class c ON c.oid = con.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'public'
		   AND con.contype IN ('p', 'u')
		   AND c.relname IN ('organizations', 'entities', 'accounts', 'memberships')
		 ORDER BY c.relname, con.conname`)
	if err != nil {
		t.Fatalf("reading constraints: %v", err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var table, name, leading string
		if err := rows.Scan(&table, &name, &leading); err != nil {
			t.Fatalf("scanning constraint: %v", err)
		}
		seen++
		if leading != tenantColumn(table) {
			t.Errorf("%s.%s leads with %q, want %q: a uniqueness constraint that does not "+
				"carry the tenant key is a cross-tenant oracle, because referential-integrity "+
				"checks bypass row-level security", table, name, leading, tenantColumn(table))
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating constraints: %v", err)
	}
	// A query that silently matched nothing would pass the loop above.
	if seen < 4 {
		t.Fatalf("found only %d uniqueness constraint(s) across the four tenant tables; the query is wrong", seen)
	}
}

// Task 1.12, design D3. Login has to answer "which organisations does this
// user belong to?" before any organisation is known -- a read that cannot go
// through a tenant transaction, because there is no tenant yet, and cannot go
// through the untenanted one either, because memberships has a policy.
//
// The mechanism is a policy scoped to a role that nothing can authenticate as,
// not a privileged role. Every assertion below is about the *bound* on that
// exception: it exists, it works, and it does not widen. Design D3's own table
// is the source of the five probes.
func TestOrgsForUserIsABoundedException(t *testing.T) {
	f := newTenantFixture(t, "vekst_tenancy_orgsforuser_test")

	// Give userA a membership in B as well, so "returns this user's rows" is
	// distinguishable from "returns one row".
	if err := inTenantTx(t, f.app, orgB, func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, 'viewer')`,
			orgB, f.userA)
		return err
	}); err != nil {
		t.Fatalf("seeding userA's membership in org B: %v", err)
	}
	var otherUser string
	if err := f.migrator.QueryRow(
		`INSERT INTO users (email) VALUES ('other@example.test') RETURNING id`).Scan(&otherUser); err != nil {
		t.Fatalf("seeding second user: %v", err)
	}

	// 1. It returns the user's memberships with NO tenant context set, called
	//    by vekst_app -- the whole point of the exception.
	rows, err := f.app.Query(`SELECT org_id::text, role FROM orgs_for_user($1) ORDER BY role`, f.userA)
	if err != nil {
		t.Fatalf("orgs_for_user with no tenant context: %v", err)
	}
	got := map[string]string{}
	for rows.Next() {
		var org, role string
		if err := rows.Scan(&org, &role); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		got[org] = role
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating: %v", err)
	}
	if len(got) != 2 || got[orgA] != "owner" || got[orgB] != "viewer" {
		t.Fatalf("orgs_for_user(userA) = %v, want {A: owner, B: viewer}", got)
	}

	// 2. It returns nothing belonging to anyone else.
	var n int
	if err := f.app.QueryRow(`SELECT count(*) FROM orgs_for_user($1)`, otherUser).Scan(&n); err != nil {
		t.Fatalf("orgs_for_user for a user with no memberships: %v", err)
	}
	if n != 0 {
		t.Fatalf("orgs_for_user returned %d row(s) for a user with no memberships", n)
	}

	// 3. The exception has not widened: memberships itself still raises for
	//    vekst_app with no context, exactly as every other tenant table does.
	//    A definer function that had been granted a bypass would show up here.
	err = f.app.QueryRow(`SELECT count(*) FROM memberships`).Scan(&n)
	if err == nil {
		t.Fatal("memberships is readable with no tenant context; the role-scoped policy has widened past its role")
	}
	if code := sqlState(err); code != "42704" {
		t.Fatalf("memberships with no context: SQLSTATE %s (%v), want 42704", code, err)
	}

	// 4. And with a context it is still filtered to that organisation, so the
	//    role-scoped policy is not being OR'd in for vekst_app.
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM memberships`).Scan(&n)
	}); err != nil {
		t.Fatalf("reading memberships under org A: %v", err)
	}
	if n != 1 {
		t.Fatalf("org A sees %d membership(s), want 1; userA's membership in org B is visible", n)
	}

	// 5. The bound that makes the rest of it acceptable: the role's entire
	//    reachable surface is the body of one function, because vekst_app
	//    cannot become it and it cannot log in.
	if _, err := f.app.Exec(`SET ROLE vekst_membership_reader`); err == nil {
		t.Fatal("vekst_app assumed vekst_membership_reader; the exception is not bounded by the role")
	}
	var canLogin, isSuper, bypassRLS bool
	if err := f.migrator.QueryRow(
		`SELECT rolcanlogin, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'vekst_membership_reader'`,
	).Scan(&canLogin, &isSuper, &bypassRLS); err != nil {
		t.Fatalf("reading vekst_membership_reader: %v", err)
	}
	if canLogin || isSuper || bypassRLS {
		t.Errorf("vekst_membership_reader has login=%v super=%v bypassrls=%v; want all false",
			canLogin, isSuper, bypassRLS)
	}

	// And memberships is still forced, which is what makes the SECURITY
	// DEFINER function subject to the policy rather than exempt from it.
	var enabled, forced bool
	if err := f.migrator.QueryRow(
		`SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE relname = 'memberships'`,
	).Scan(&enabled, &forced); err != nil {
		t.Fatalf("reading memberships RLS flags: %v", err)
	}
	if !enabled || !forced {
		t.Errorf("memberships: rowsecurity=%v forced=%v; want both true", enabled, forced)
	}
}

// The reporting currency has exactly one home in the schema.
//
// Design §1 argues this at length and migration 00004 implements it -- but
// nothing enforced it, and the failure it guards against is silent by
// construction. Two unconstrained copies of the currency a report converts
// into can disagree with no error, and whichever copy the query happens to
// read wins. `ARCHITECTURE.md` §5.5 sketched `base_currency` on `entities` as
// well, so the migration that puts it back is a plausible one to write.
//
// This asserts on the reporting currency specifically, not on every column
// whose name contains "currency": `accounts.currency` is the denomination an
// account is held in, which is a different fact and legitimately per-account.
func TestTheReportingCurrencyHasOneHome(t *testing.T) {
	f := newTenantFixture(t, "vekst_tenancy_currency_test")

	rows, err := f.migrator.Query(`
		SELECT c.relname
		  FROM pg_attribute a
		  JOIN pg_class c ON c.oid = a.attrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'public'
		   AND c.relkind IN ('r', 'p', 'm', 'v')
		   AND a.attname = 'base_currency'
		   AND a.attnum > 0
		   AND NOT a.attisdropped
		 ORDER BY c.relname`)
	if err != nil {
		t.Fatalf("looking for base_currency columns: %v", err)
	}
	defer rows.Close()

	var carriers []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		carriers = append(carriers, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating: %v", err)
	}

	if len(carriers) != 1 || carriers[0] != "organizations" {
		t.Fatalf("base_currency lives on %v, want exactly [organizations]. Two copies of the "+
			"currency a report converts into can disagree with no error; if a second home is "+
			"genuinely needed, the change that adds it must also add the rule for which one "+
			"applies (design §1)", carriers)
	}
}

// An entity-scoped row cannot omit its entity.
//
// `entity_id` is populated from this first migration even though the Demo
// gives each organisation exactly one entity, so a holding customer needs no
// migration of existing data -- only new rows. That is worth nothing if the
// column is nullable, and "make it nullable to unblock an import" is an
// ordinary-looking thing to propose.
func TestAnAccountCannotOmitItsEntity(t *testing.T) {
	f := newTenantFixture(t, "vekst_tenancy_entityreq_test")

	err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO accounts (org_id, name, currency) VALUES ($1, 'no entity', 'NOK')`, orgA)
		return err
	})
	if err == nil {
		t.Fatal("an account was inserted with no entity_id")
	}
	if code := sqlState(err); code != "23502" {
		t.Fatalf("account with no entity_id: SQLSTATE %s (%v), want 23502 not_null_violation", code, err)
	}
}
