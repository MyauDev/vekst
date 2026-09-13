package migrate

import (
	"database/sql"
	"testing"
)

// Proof that migration 00006's policies behave, rather than merely exist.
//
// Two tables with deliberately different shapes, and the tests come in two
// shapes to match. `classification_rules` is shared and tenant at once, like
// `categories`: most rules belong to a country or a bank, so every
// organisation must read them and none may write them. `vendors` is an
// ordinary tenant table, because memory is earned by one organisation's own
// decisions and is shared with nobody.
//
// Everything runs as vekst_app against a schema migrated by a non-superuser
// vekst_migrator, because that is the only arrangement in which row-level
// security means anything.

// The seed's own number, asserted rather than counted, so that a change to the
// rule set has to come here and say so. eval/emit.py prints it on every run.
const (
	rulesetV     = "v1"
	seededRules  = 71 // 41 BY + 21 KZ + 9 PL
	sharedLeafBY = "0404"
)

// Task 6.6, first half.
func TestEveryOrganisationSeesEveryTemplateRule(t *testing.T) {
	f := newTenantFixture(t, "vekst_rules_shared_read_test")

	for _, org := range []string{orgA, orgB} {
		var n int
		if err := inTenantTx(t, f.app, org, func(tx *sql.Tx) error {
			return tx.QueryRow(`
				SELECT count(*) FROM classification_rules
				 WHERE taxonomy_version = $1 AND ruleset_version = $2 AND org_id IS NULL`,
				taxonomyV, rulesetV).Scan(&n)
		}); err != nil {
			t.Fatalf("org %s reading the template rules: %v", org, err)
		}
		if n != seededRules {
			t.Errorf("org %s sees %d template rules, want %d -- a rule about how "+
				"Belarusian statements are worded is not one customer's property",
				org, n, seededRules)
		}
	}
}

// Task 6.6, second half. A template rule belongs to nobody, and the
// application may read one but never create, alter or remove one. That is the
// whole reason the read policy and the write policy are separate.
func TestAnOrganisationCannotWriteATemplateRule(t *testing.T) {
	f := newTenantFixture(t, "vekst_rules_shared_write_test")

	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO classification_rules
			       (taxonomy_version, ruleset_version, org_id, scope, priority, matcher, category_id)
			SELECT $1, $2, NULL, 'country:BY', 9001,
			       '{"all": [{"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, id
			  FROM categories
			 WHERE taxonomy_version = $1 AND org_id IS NULL AND code = $3`,
			taxonomyV, rulesetV, sharedLeafBY)
		return err
	}); err == nil {
		t.Fatal("vekst_app created a template rule; the write policy admits only its own")
	} else if got := sqlState(err); got != "42501" {
		t.Errorf("SQLSTATE = %s, want 42501 (insufficient privilege)", got)
	}

	// And it cannot silently retire one either. RLS filters UPDATE rather
	// than refusing it, so the tell is a zero row count, not an error -- and
	// the rows have to be unchanged for everybody afterwards.
	var affected int64
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`UPDATE classification_rules SET active = false WHERE org_id IS NULL`)
		if err != nil {
			return err
		}
		affected, err = res.RowsAffected()
		return err
	}); err != nil {
		t.Fatalf("updating template rules: %v", err)
	}
	if affected != 0 {
		t.Errorf("vekst_app deactivated %d template rules", affected)
	}

	var live int
	if err := inTenantTx(t, f.app, orgB, func(tx *sql.Tx) error {
		return tx.QueryRow(
			`SELECT count(*) FROM classification_rules WHERE org_id IS NULL AND active`).Scan(&live)
	}); err != nil {
		t.Fatalf("org B reading back: %v", err)
	}
	if live != seededRules {
		t.Errorf("%d template rules still active for org B, want %d", live, seededRules)
	}
}

// Task 6.6, third half. An organisation's own rule is an ordinary tenant row.
func TestOneOrganisationCannotSeeAnothersRules(t *testing.T) {
	f := newTenantFixture(t, "vekst_rules_cross_read_test")

	bRule := insertOrgRule(t, f, orgB, 1)

	var n int
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM classification_rules WHERE id = $1`, bRule).Scan(&n)
	}); err != nil {
		t.Fatalf("org A reading: %v", err)
	}
	if n != 0 {
		t.Error("org A can see a rule belonging to org B")
	}

	// Isolation must not cost A the rules that belong to everyone.
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(
			`SELECT count(*) FROM classification_rules WHERE org_id IS NULL`).Scan(&n)
	}); err != nil {
		t.Fatalf("org A reading the templates: %v", err)
	}
	if n != seededRules {
		t.Errorf("org A sees %d template rules, want %d", n, seededRules)
	}

	var affected int64
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		res, err := tx.Exec(`DELETE FROM classification_rules WHERE id = $1`, bRule)
		if err != nil {
			return err
		}
		affected, err = res.RowsAffected()
		return err
	}); err != nil {
		t.Fatalf("org A deleting B's rule: %v", err)
	}
	if affected != 0 {
		t.Error("org A deleted a rule belonging to org B")
	}
}

// Both organisations may use the same priority number, because priority is
// unique per owner rather than across owners. Which of the two a transaction
// gets is settled by the query's ordering, not by the index -- see
// core/internal/db/query/classify.sql.
func TestTwoOrganisationsMayReuseAPriority(t *testing.T) {
	f := newTenantFixture(t, "vekst_rules_priority_test")

	insertOrgRule(t, f, orgA, 7)
	insertOrgRule(t, f, orgB, 7)

	// And a template rule already holds 7, which is the case that would fail
	// if the index tried to be unique across owners.
	var templates int
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(
			`SELECT count(*) FROM classification_rules WHERE org_id IS NULL AND priority = 7`).Scan(&templates)
	}); err != nil {
		t.Fatalf("reading template priority 7: %v", err)
	}
	if templates != 1 {
		t.Fatalf("expected exactly one template rule at priority 7, found %d", templates)
	}
}

// One organisation may not hold the same priority twice: a tie would make the
// engine's answer depend on which row came back first.
func TestOneOrganisationCannotReuseItsOwnPriority(t *testing.T) {
	f := newTenantFixture(t, "vekst_rules_priority_dup_test")

	insertOrgRule(t, f, orgA, 11)
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return insertRule(tx, orgA, 11)
	}); err == nil {
		t.Fatal("org A holds priority 11 twice")
	} else if got := sqlState(err); got != "23505" {
		t.Errorf("SQLSTATE = %s, want 23505 (unique violation)", got)
	}
}

// A rule may target only a leaf that is not computed. A section's figure is
// already the sum of its children, and GM is NET SALES minus CS -- an amount
// landing on either is counted twice.
func TestARuleCannotTargetASectionOrAComputedLine(t *testing.T) {
	f := newTenantFixture(t, "vekst_rules_target_test")

	for _, tc := range []struct{ what, code string }{
		{"a section", "04"},
		{"a computed line", "91"},
	} {
		err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				INSERT INTO classification_rules
				       (taxonomy_version, ruleset_version, org_id, scope, priority, matcher, category_id)
				SELECT $1, $2, $3, 'org', 8000,
				       '{"all": [{"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, id
				  FROM categories
				 WHERE taxonomy_version = $1 AND org_id IS NULL AND code = $4`,
				taxonomyV, rulesetV, orgA, tc.code)
			return err
		})
		if err == nil {
			t.Errorf("a rule was allowed to target %s (%s)", tc.what, tc.code)
			continue
		}
		if got := sqlState(err); got != "23514" {
			t.Errorf("targeting %s: SQLSTATE = %s, want 23514 (check violation)", tc.what, got)
		}
	}
}

// A matcher with no conditions would fire on every transaction. The database
// says so rather than trusting whatever wrote the row.
func TestAMatcherMustHaveConditions(t *testing.T) {
	f := newTenantFixture(t, "vekst_rules_matcher_test")

	for _, matcher := range []string{`{"all": []}`, `{}`, `{"all": "expense"}`} {
		err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				INSERT INTO classification_rules
				       (taxonomy_version, ruleset_version, org_id, scope, priority, matcher, category_id)
				SELECT $1, $2, $3, 'org', 8100, $4::jsonb, id
				  FROM categories
				 WHERE taxonomy_version = $1 AND org_id IS NULL AND code = $5`,
				taxonomyV, rulesetV, orgA, matcher, sharedLeafBY)
			return err
		})
		if err == nil {
			t.Errorf("matcher %s was accepted; it would fire on every transaction", matcher)
			continue
		}
		if got := sqlState(err); got != "23514" {
			t.Errorf("matcher %s: SQLSTATE = %s, want 23514 (check violation)", matcher, got)
		}
	}
}

// ---------------------------------------------------------------------------
// vendors -- an ordinary tenant table. Task 6.5.
// ---------------------------------------------------------------------------

func TestOneOrganisationCannotReadAnothersVendorMemory(t *testing.T) {
	f := newTenantFixture(t, "vekst_vendors_cross_read_test")

	bVendor := insertVendor(t, f, orgB, "tax:220340017991")

	var n int
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM vendors WHERE id = $1`, bVendor).Scan(&n)
	}); err != nil {
		t.Fatalf("org A reading: %v", err)
	}
	if n != 0 {
		t.Error("org A can read org B's vendor memory -- a decision B made about its own supplier")
	}

	// And A sees nothing at all, not merely not that row: vendors has no
	// shared rows, so an empty table is the correct view for a new customer.
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM vendors`).Scan(&n)
	}); err != nil {
		t.Fatalf("org A counting: %v", err)
	}
	if n != 0 {
		t.Errorf("org A sees %d vendor rows, want none", n)
	}
}

func TestOneOrganisationCannotWriteAnothersVendorMemory(t *testing.T) {
	f := newTenantFixture(t, "vekst_vendors_cross_write_test")

	bVendor := insertVendor(t, f, orgB, "tax:220340017991")

	// An UPDATE that reaches no row is the correct outcome, and it is
	// indistinguishable from the row not existing -- which is the point.
	var affected int64
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`UPDATE vendors SET display_name = 'seen by A' WHERE id = $1`, bVendor)
		if err != nil {
			return err
		}
		affected, err = res.RowsAffected()
		return err
	}); err != nil {
		t.Fatalf("org A updating: %v", err)
	}
	if affected != 0 {
		t.Error("org A rewrote org B's vendor memory")
	}

	var name string
	if err := inTenantTx(t, f.app, orgB, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT display_name FROM vendors WHERE id = $1`, bVendor).Scan(&name)
	}); err != nil {
		t.Fatalf("org B reading back: %v", err)
	}
	if name == "seen by A" {
		t.Error("org B's vendor memory was changed by org A")
	}

	// A cannot write a row into B's name either.
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO vendors (org_id, key, key_version, display_name, category_id)
			SELECT $1, 'tax:1', 'v1', 'planted', id
			  FROM categories WHERE taxonomy_version = $2 AND org_id IS NULL AND code = $3`,
			orgB, taxonomyV, sharedLeafBY)
		return err
	}); err == nil {
		t.Error("org A inserted a vendor row owned by org B")
	} else if got := sqlState(err); got != "42501" {
		t.Errorf("SQLSTATE = %s, want 42501 (insufficient privilege)", got)
	}
}

// Both organisations may remember the same counterparty, each with its own
// answer. The uniqueness constraint takes org_id as its leading column, so one
// customer's decision is not an oracle for another's.
func TestTwoOrganisationsMayRememberTheSameCounterparty(t *testing.T) {
	f := newTenantFixture(t, "vekst_vendors_same_key_test")

	insertVendor(t, f, orgA, "tax:220340017991")
	insertVendor(t, f, orgB, "tax:220340017991")

	for _, org := range []string{orgA, orgB} {
		var n int
		if err := inTenantTx(t, f.app, org, func(tx *sql.Tx) error {
			return tx.QueryRow(
				`SELECT count(*) FROM vendors WHERE key = 'tax:220340017991'`).Scan(&n)
		}); err != nil {
			t.Fatalf("org %s reading: %v", org, err)
		}
		if n != 1 {
			t.Errorf("org %s sees %d rows for one counterparty, want 1", org, n)
		}
	}
}

// A key produced by an older counterparty_key() does not mean what today's
// does, so the version is part of the identity rather than a note beside it.
func TestVendorMemoryIsKeyedByItsKeyVersion(t *testing.T) {
	f := newTenantFixture(t, "vekst_vendors_key_version_test")

	insertVendor(t, f, orgA, "tax:220340017991")
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO vendors (org_id, key, key_version, display_name, category_id)
			SELECT $1, 'tax:220340017991', 'v2', 'later', id
			  FROM categories WHERE taxonomy_version = $2 AND org_id IS NULL AND code = $3`,
			orgA, taxonomyV, sharedLeafBY)
		return err
	}); err != nil {
		t.Fatalf("the same key under a later version must be a different row: %v", err)
	}

	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO vendors (org_id, key, key_version, display_name, category_id)
			SELECT $1, 'tax:220340017991', 'v1', 'duplicate', id
			  FROM categories WHERE taxonomy_version = $2 AND org_id IS NULL AND code = $3`,
			orgA, taxonomyV, sharedLeafBY)
		return err
	}); err == nil {
		t.Error("the same key twice under one version was accepted")
	} else if got := sqlState(err); got != "23505" {
		t.Errorf("SQLSTATE = %s, want 23505 (unique violation)", got)
	}
}

// A category belonging to somebody else must be refused with the same error as
// one that does not exist, or the constraint becomes an existence oracle.
func TestAVendorCannotPointAtAnotherOrganisationsCategory(t *testing.T) {
	f := newTenantFixture(t, "vekst_vendors_oracle_test")

	var bLeaf string
	if err := inTenantTx(t, f.app, orgB, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO categories (taxonomy_version, org_id, scope, code, parent_id, level, name, is_leaf)
			SELECT $1, $2, 'org', '040198', id, 3, 'B private leaf', true
			  FROM categories WHERE taxonomy_version = $1 AND org_id IS NULL AND code = '0401'
			RETURNING id`, taxonomyV, orgB).Scan(&bLeaf)
	}); err != nil {
		t.Fatalf("org B creating its own leaf: %v", err)
	}

	insert := func(categoryID string) error {
		return inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				INSERT INTO vendors (org_id, key, key_version, display_name, category_id)
				VALUES ($1, 'tax:1', 'v1', 'x', $2)`, orgA, categoryID)
			return err
		})
	}

	somebodyElses := insert(bLeaf)
	nobodys := insert("00000000-0000-0000-0000-000000000000")
	if somebodyElses == nil {
		t.Fatal("org A pointed a vendor row at org B's category")
	}
	if nobodys == nil {
		t.Fatal("org A pointed a vendor row at a category that does not exist")
	}
	if sqlState(somebodyElses) != sqlState(nobodys) {
		t.Errorf("a category belonging to B fails with %s and a missing one with %s; "+
			"the difference tells org A that B's row exists",
			sqlState(somebodyElses), sqlState(nobodys))
	}
}

// ---------------------------------------------------------------------------

func insertRule(tx *sql.Tx, org string, priority int) error {
	_, err := tx.Exec(`
		INSERT INTO classification_rules
		       (taxonomy_version, ruleset_version, org_id, scope, priority, matcher, category_id)
		SELECT $1, $2, $3, 'org', $4,
		       '{"all": [{"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, id
		  FROM categories
		 WHERE taxonomy_version = $1 AND org_id IS NULL AND code = $5`,
		taxonomyV, rulesetV, org, priority, sharedLeafBY)
	return err
}

func insertOrgRule(t *testing.T, f *tenantFixture, org string, priority int) string {
	t.Helper()
	var id string
	if err := inTenantTx(t, f.app, org, func(tx *sql.Tx) error {
		if err := insertRule(tx, org, priority); err != nil {
			return err
		}
		return tx.QueryRow(
			`SELECT id FROM classification_rules WHERE org_id = $1 AND priority = $2`,
			org, priority).Scan(&id)
	}); err != nil {
		t.Fatalf("org %s creating a rule at priority %d: %v", org, priority, err)
	}
	return id
}

func insertVendor(t *testing.T, f *tenantFixture, org, key string) string {
	t.Helper()
	var id string
	if err := inTenantTx(t, f.app, org, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO vendors (org_id, key, key_version, display_name, category_id)
			SELECT $1, $2, 'v1', 'Remembered', id
			  FROM categories WHERE taxonomy_version = $3 AND org_id IS NULL AND code = $4
			RETURNING id`, org, key, taxonomyV, sharedLeafBY).Scan(&id)
	}); err != nil {
		t.Fatalf("org %s remembering %s: %v", org, key, err)
	}
	return id
}
