package migrate

import (
	"database/sql"
	"testing"
)

// Proof that migration 00005's policies behave, rather than merely exist.
//
// Same shape as tenancy_test.go and for the same reason: every assertion runs
// as vekst_app against a schema migrated by a non-superuser vekst_migrator,
// because that is the only arrangement in which row-level security means
// anything.
//
// What makes this table different from the four in 00004 is that some of its
// rows belong to nobody. The accounting skeleton — NET SALES, OPEX, and the
// arithmetic that turns them into GM — must read the same for every customer,
// while a leaf like "Merch for employees" belongs to one. So the tests below
// come in pairs: a shared row is readable by everyone and writable by no one,
// and an owned row behaves exactly as any tenant row does.

// The seed's own numbers, asserted rather than counted, so that a change to the
// taxonomy has to come here and say so. They come from eval/emit.py, which
// prints them on every run.
const (
	taxonomyV       = "v1"
	seededRows      = 46 // 41 shared nodes + 5 computed lines
	seededLeaves    = 21
	seededComputed  = 5
	seededAllocable = 2 // the two payroll buckets, and nothing else
)

func TestSharedCategoriesAreVisibleToEveryOrganisation(t *testing.T) {
	f := newTenantFixture(t, "vekst_taxonomy_shared_read_test")

	for _, org := range []string{orgA, orgB} {
		var n int
		if err := inTenantTx(t, f.app, org, func(tx *sql.Tx) error {
			return tx.QueryRow(
				`SELECT count(*) FROM categories WHERE taxonomy_version = $1`, taxonomyV).Scan(&n)
		}); err != nil {
			t.Fatalf("org %s reading the taxonomy: %v", org, err)
		}
		if n != seededRows {
			t.Errorf("org %s sees %d categories, want %d -- the shared skeleton must read "+
				"identically for every customer, or two reports stop being comparable", org, n, seededRows)
		}
	}
}

func TestOneOrganisationCannotSeeAnothersCategories(t *testing.T) {
	f := newTenantFixture(t, "vekst_taxonomy_cross_read_test")

	// B adds a leaf of its own under a shared parent.
	var bLeaf string
	if err := inTenantTx(t, f.app, orgB, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO categories (taxonomy_version, org_id, scope, code, parent_id, level, name, is_leaf)
			SELECT $1, $2, 'org', '040199', id, 3, 'B private leaf', true
			  FROM categories WHERE taxonomy_version = $1 AND code = '0401'
			RETURNING id`, taxonomyV, orgB).Scan(&bLeaf)
	}); err != nil {
		t.Fatalf("org B creating its own leaf: %v", err)
	}

	var n int
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM categories WHERE id = $1`, bLeaf).Scan(&n)
	}); err != nil {
		t.Fatalf("org A reading: %v", err)
	}
	if n != 0 {
		t.Error("org A can see a category belonging to org B")
	}

	// And A still sees the whole shared tree: isolation must not cost it the
	// rows that belong to everyone.
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(
			`SELECT count(*) FROM categories WHERE taxonomy_version = $1`, taxonomyV).Scan(&n)
	}); err != nil {
		t.Fatalf("org A reading the taxonomy: %v", err)
	}
	if n != seededRows {
		t.Errorf("org A sees %d categories, want the %d shared ones", n, seededRows)
	}
}

func TestOneOrganisationCannotWriteAnothersCategories(t *testing.T) {
	f := newTenantFixture(t, "vekst_taxonomy_cross_write_test")

	var bLeaf string
	if err := inTenantTx(t, f.app, orgB, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO categories (taxonomy_version, org_id, scope, code, level, name, is_leaf)
			VALUES ($1, $2, 'org', '040199', 3, 'B private leaf', true)
			RETURNING id`, taxonomyV, orgB).Scan(&bLeaf)
	}); err != nil {
		t.Fatalf("org B creating its own leaf: %v", err)
	}

	for _, tc := range []struct {
		name string
		stmt string
	}{
		{"update", `UPDATE categories SET name = 'stolen' WHERE id = $1`},
		{"delete", `DELETE FROM categories WHERE id = $1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var affected int64
			if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
				res, err := tx.Exec(tc.stmt, bLeaf)
				if err != nil {
					return err
				}
				affected, err = res.RowsAffected()
				return err
			}); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			// Zero rows, not an error: row-level security filters writes
			// silently, which is why db.ExactlyOneRow exists.
			if affected != 0 {
				t.Errorf("org A %sd %d of org B's categories", tc.name, affected)
			}
		})
	}

	// B's row is untouched.
	var name string
	if err := inTenantTx(t, f.app, orgB, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT name FROM categories WHERE id = $1`, bLeaf).Scan(&name)
	}); err != nil {
		t.Fatalf("org B reading back: %v", err)
	}
	if name != "B private leaf" {
		t.Errorf("org B's category now reads %q", name)
	}
}

// The reason read and write are separate policies. A shared row is every
// customer's; letting one of them edit it would change GM's formula for all of
// them at once, and the report would be wrong for everybody rather than for one
// tenant.
func TestSharedCategoriesAreReadOnlyToTheApplication(t *testing.T) {
	f := newTenantFixture(t, "vekst_taxonomy_shared_write_test")

	// Even the migrator reads this through a tenant transaction. FORCE applies
	// to the owner, and the read policy calls app_current_org(), which raises
	// 42704 when no context is set -- so a shared row is not readable outside
	// one either. Fail-closed, deliberately: an untenanted read returning the
	// shared skeleton would be a query that quietly half-works.
	var sharedID string
	if err := inTenantTx(t, f.migrator, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(
			`SELECT id FROM categories WHERE taxonomy_version = $1 AND code = '91'`,
			taxonomyV).Scan(&sharedID)
	}); err != nil {
		t.Fatalf("finding a shared row: %v", err)
	}

	t.Run("cannot update", func(t *testing.T) {
		var affected int64
		if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			res, err := tx.Exec(
				`UPDATE categories SET formula = 'NET SALES' WHERE id = $1`, sharedID)
			if err != nil {
				return err
			}
			affected, err = res.RowsAffected()
			return err
		}); err != nil {
			t.Fatalf("update: %v", err)
		}
		if affected != 0 {
			t.Error("the application rewrote a shared category; GM's formula is now one " +
				"customer's opinion for every customer")
		}
	})

	t.Run("cannot delete", func(t *testing.T) {
		var affected int64
		if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			res, err := tx.Exec(`DELETE FROM categories WHERE id = $1`, sharedID)
			if err != nil {
				return err
			}
			affected, err = res.RowsAffected()
			return err
		}); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if affected != 0 {
			t.Error("the application deleted a shared category")
		}
	})

	t.Run("cannot create", func(t *testing.T) {
		err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				INSERT INTO categories (taxonomy_version, org_id, scope, code, level, name, is_leaf)
				VALUES ($1, NULL, 'global', '99', 1, 'Forged shared row', true)`, taxonomyV)
			return err
		})
		if err == nil {
			t.Fatal("the application created a row belonging to every organisation")
		}
		// 42501 is insufficient_privilege: the WITH CHECK rejected it.
		if got := sqlState(err); got != "42501" {
			t.Errorf("insert failed with SQLSTATE %q, want 42501 from the write policy", got)
		}
	})
}

// The constraint trigger that replaces a composite foreign key. The legitimate
// case -- a leaf under a shared parent -- is what a composite key cannot
// express, because the parent's org_id is NULL and never equals the child's.
func TestCategoryParentMustBeSharedOrMine(t *testing.T) {
	f := newTenantFixture(t, "vekst_taxonomy_parent_test")

	var sharedParent, bParent string
	if err := inTenantTx(t, f.migrator, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(
			`SELECT id FROM categories WHERE taxonomy_version = $1 AND code = '0401'`,
			taxonomyV).Scan(&sharedParent)
	}); err != nil {
		t.Fatalf("finding a shared parent: %v", err)
	}
	if err := inTenantTx(t, f.app, orgB, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO categories (taxonomy_version, org_id, scope, code, level, name, is_leaf)
			VALUES ($1, $2, 'org', '040198', 3, 'B branch', false)
			RETURNING id`, taxonomyV, orgB).Scan(&bParent)
	}); err != nil {
		t.Fatalf("org B creating a branch: %v", err)
	}

	t.Run("a shared parent is allowed", func(t *testing.T) {
		if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				INSERT INTO categories (taxonomy_version, org_id, scope, code, parent_id, level, name, is_leaf)
				VALUES ($1, $2, 'org', '040197', $3, 3, 'A leaf under shared', true)`,
				taxonomyV, orgA, sharedParent)
			return err
		}); err != nil {
			t.Fatalf("a leaf under a shared parent was refused: %v -- this is the case a "+
				"composite foreign key cannot express, and the whole reason for the trigger", err)
		}
	})

	// Another organisation's parent and a parent that does not exist must fail
	// identically. A different message for each would confirm the existence of
	// a row the caller may not read.
	t.Run("another organisation's parent is refused like a missing one", func(t *testing.T) {
		var stateForeign, stateMissing string
		err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				INSERT INTO categories (taxonomy_version, org_id, scope, code, parent_id, level, name, is_leaf)
				VALUES ($1, $2, 'org', '040196', $3, 3, 'A leaf under B', true)`,
				taxonomyV, orgA, bParent)
			return err
		})
		if err == nil {
			t.Fatal("a category was hung under another organisation's parent")
		}
		stateForeign = sqlState(err)

		err = inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				INSERT INTO categories (taxonomy_version, org_id, scope, code, parent_id, level, name, is_leaf)
				VALUES ($1, $2, 'org', '040195', $3, 3, 'A leaf under nothing', true)`,
				taxonomyV, orgA, "dddddddd-dddd-dddd-dddd-dddddddddddd")
			return err
		})
		if err == nil {
			t.Fatal("a category was hung under a parent that does not exist")
		}
		stateMissing = sqlState(err)

		if stateForeign != stateMissing {
			t.Errorf("naming another organisation's parent gives SQLSTATE %q and naming a "+
				"missing one gives %q; the difference tells a caller that the first one exists",
				stateForeign, stateMissing)
		}
	})
}

// The seed goes in before the policies exist. Afterwards even the migrator is
// refused, which is what makes the ordering load-bearing rather than tidy.
func TestASharedRowCannotBeAddedOnceThePoliciesExist(t *testing.T) {
	f := newTenantFixture(t, "vekst_taxonomy_seed_order_test")

	_, err := f.migrator.Exec(`
		INSERT INTO categories (taxonomy_version, org_id, scope, code, level, name, is_leaf)
		VALUES ($1, NULL, 'global', '98', 1, 'Late shared row', true)`, taxonomyV)
	if err == nil {
		t.Fatal("the migrator inserted a shared row after FORCE ROW LEVEL SECURITY; either " +
			"FORCE is not set or the write policy admits an unowned row, and migration 005's " +
			"seed ordering is then decorative")
	}
}

func TestOnlyClassifiableCategoriesCanReceiveATransaction(t *testing.T) {
	f := newTenantFixture(t, "vekst_taxonomy_classifiable_test")

	var total, classifiable, computed, allocable int
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT count(*),
			       count(*) FILTER (WHERE is_leaf AND NOT is_computed),
			       count(*) FILTER (WHERE is_computed),
			       count(*) FILTER (WHERE requires_allocation)
			  FROM categories WHERE taxonomy_version = $1`, taxonomyV).
			Scan(&total, &classifiable, &computed, &allocable)
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}

	if total != seededRows {
		t.Errorf("taxonomy holds %d rows, want %d", total, seededRows)
	}
	if classifiable != seededLeaves {
		t.Errorf("%d categories can receive a transaction, want %d", classifiable, seededLeaves)
	}
	if computed != seededComputed {
		t.Errorf("%d computed lines, want %d (GM, NM, CM, IBT, NI)", computed, seededComputed)
	}
	if allocable != seededAllocable {
		t.Errorf("%d categories require allocation, want %d -- the two payroll buckets and "+
			"nothing else", allocable, seededAllocable)
	}

	// The property behind the count: a computed line is never a leaf, so
	// ClassifiableCategories cannot return one however the filter is written.
	var overlap int
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(
			`SELECT count(*) FROM categories WHERE is_computed AND is_leaf`).Scan(&overlap)
	}); err != nil {
		t.Fatalf("checking the overlap: %v", err)
	}
	if overlap != 0 {
		t.Errorf("%d categories are both computed and a leaf; a transaction could land in a "+
			"line that already sums it", overlap)
	}
}

// A tree, not a list: every non-root resolves to a parent, and the roots are
// the nine report sections plus the five computed lines.
func TestTheSeededTreeIsWellFormed(t *testing.T) {
	f := newTenantFixture(t, "vekst_taxonomy_shape_test")

	var orphans int
	if err := inTenantTx(t, f.migrator, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT count(*) FROM categories c
			 WHERE c.taxonomy_version = $1 AND c.level > 1 AND c.parent_id IS NULL`,
			taxonomyV).Scan(&orphans)
	}); err != nil {
		t.Fatalf("counting orphans: %v", err)
	}
	if orphans != 0 {
		t.Errorf("%d categories below the top level have no parent", orphans)
	}

	var badLevel int
	if err := inTenantTx(t, f.migrator, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT count(*) FROM categories c JOIN categories p ON p.id = c.parent_id
			 WHERE c.level <> p.level + 1`).Scan(&badLevel)
	}); err != nil {
		t.Fatalf("checking levels: %v", err)
	}
	if badLevel != 0 {
		t.Errorf("%d categories sit at a level that is not their parent's plus one", badLevel)
	}

	var leafWithChildren int
	if err := inTenantTx(t, f.migrator, orgA, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT count(DISTINCT p.id) FROM categories p JOIN categories c ON c.parent_id = p.id
			 WHERE p.is_leaf`).Scan(&leafWithChildren)
	}); err != nil {
		t.Fatalf("checking leaves: %v", err)
	}
	if leafWithChildren != 0 {
		t.Errorf("%d categories are marked as leaves but have children", leafWithChildren)
	}
}
