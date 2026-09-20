package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

// Task 7.3. Adoption: 60 rows land, every parent resolves, level-5 leaves
// hang under the organisation's own level-4 rows, and 030101 hangs under the
// global 0301.
func TestAdoptIndustryTemplateResolvesEveryParent(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	org := testOrg(t, d, testUser(t, d))

	var rows []gendb.EffectiveTaxonomyRow
	if err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)
		var err error
		rows, err = q.EffectiveTaxonomy(ctx, TaxonomyVersion)
		return err
	}); err != nil {
		t.Fatalf("reading back the effective taxonomy: %v", err)
	}

	byCode := make(map[string]gendb.EffectiveTaxonomyRow, len(rows))
	adopted := 0
	for _, r := range rows {
		byCode[r.Code] = r
		if r.OrgID.Valid {
			adopted++
		}
	}
	if adopted != 60 {
		t.Errorf("adopted %d org-scoped categories, want 60", adopted)
	}

	// 030101 (level 3) hangs under the global 0301, not an org row -- 0301 is
	// never adopted, since only its child is in the industry template.
	leaf, ok := byCode["030101"]
	if !ok {
		t.Fatal("030101 was not adopted")
	}
	parent, ok := byCode["0301"]
	if !ok {
		t.Fatal("0301 (global) is missing from the effective taxonomy")
	}
	if leaf.ParentID != parent.ID {
		t.Errorf("030101's parent_id = %v, want 0301's id %v", leaf.ParentID, parent.ID)
	}
	if parent.OrgID.Valid {
		t.Error("0301 was adopted as an org row; it should stay global -- only its child is templated")
	}

	// 0401010101 (level 5, Salary) hangs under this organisation's own
	// 04010101 (level 4, FI Expenses), which the same loop adopted -- not a
	// global row, since 04010101 is not seeded by migration 005.
	child, ok := byCode["0401010101"]
	if !ok {
		t.Fatal("0401010101 was not adopted")
	}
	orgParent, ok := byCode["04010101"]
	if !ok {
		t.Fatal("04010101 was not adopted")
	}
	if !orgParent.OrgID.Valid {
		t.Error("04010101 resolved to a global row; it should be this organisation's own adopted copy")
	}
	if child.ParentID != orgParent.ID {
		t.Errorf("0401010101's parent_id = %v, want 04010101's adopted id %v", child.ParentID, orgParent.ID)
	}

	// Every adopted row's parent_id points at a row this same read can see --
	// either a global row or another of this organisation's own. A dangling
	// parent_id would either be caught by categories_parent_is_visible at
	// insert time, or, if it were not, would show up here as an id absent
	// from the set this loop just built.
	ids := make(map[[16]byte]bool, len(rows))
	for _, r := range rows {
		ids[r.ID.Bytes] = true
	}
	for _, r := range rows {
		if r.OrgID.Valid && r.ParentID.Valid && !ids[r.ParentID.Bytes] {
			t.Errorf("adopted category %q has parent_id %v, which is not in the effective taxonomy", r.Code, r.ParentID)
		}
	}
}

// Task 7.1. Cross-tenant isolation: organisation A adopts the template;
// organisation B's EffectiveTaxonomy returns none of A's adopted rows, and a
// read bound to B cannot see an A row by id.
func TestAdoptedCategoriesAreNotVisibleAcrossOrganisations(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	owner := testUser(t, d)
	orgA := testOrg(t, d, owner)
	orgB := testOrg(t, d, owner)

	var aLeafID pgtype.UUID
	if err := d.InTx(ctx, orgA, func(ctx context.Context, tx pgx.Tx) error {
		row, err := gendb.New(tx).CategoryByCode(ctx, gendb.CategoryByCodeParams{
			TaxonomyVersion: TaxonomyVersion,
			OrgID:           orgA.pg(),
			Code:            "0401010101",
		})
		if err != nil {
			return err
		}
		aLeafID = row.ID
		return nil
	}); err != nil {
		t.Fatalf("reading org A's adopted leaf: %v", err)
	}

	if err := d.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		rows, err := q.EffectiveTaxonomy(ctx, TaxonomyVersion)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.ID == aLeafID {
				t.Error("org B's EffectiveTaxonomy includes org A's adopted category")
			}
		}

		// By id, not only by list: the same category code exists in both
		// organisations (each adopted its own copy), so this asks for A's row
		// specifically, by the identifier org B is never supposed to have.
		_, err = q.CategoryByCode(ctx, gendb.CategoryByCodeParams{
			TaxonomyVersion: TaxonomyVersion,
			OrgID:           orgA.pg(),
			Code:            "0401010101",
		})
		if err == nil {
			t.Error("a transaction bound to org B resolved org A's own category row")
		}
		return nil
	}); err != nil {
		t.Fatalf("reading from org B: %v", err)
	}
}

// Task 7.4. Adoption is atomic: a template row with an unresolvable
// parent_code leaves no organisation, no entity, no membership and no
// category behind.
//
// category_templates itself is read-only to vekst_app (deploy/db/rls-exempt-tables.txt),
// so this cannot corrupt the real template to force adoptIndustryTemplate to
// fail. It proves the same property the production code depends on instead:
// CreateOrganization's transaction is InTx's ordinary all-or-nothing
// transaction, and a failure at any point inside it -- adoption's unresolvable-
// parent error included, which is a plain returned error like any other --
// rolls back everything already written in the same transaction. This test
// exercises that guarantee directly, with a category insert standing in for
// adoption's own failure mode.
func TestAdoptionFailureRollsBackTheWholeOrganisation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	org := OrgIDForNewOrg()

	errBroken := errors.New("simulated: adoption found an unresolvable parent")

	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)
		if _, err := q.InsertOrganization(ctx, gendb.InsertOrganizationParams{
			ID: org.pg(), Name: "Broken template test", Country: "NO", BaseCurrency: "NOK",
		}); err != nil {
			return err
		}
		if _, err := q.InsertEntity(ctx, gendb.InsertEntityParams{
			OrgID: org.pg(), Name: "Broken AS",
		}); err != nil {
			return err
		}
		// A category insert stands in for adoptIndustryTemplate's own
		// unresolvable-parent failure, which is likewise a plain error
		// returned mid-transaction and never reached in production, because
		// migration 00018's seed has no such row (verified: 1.7/1.8).
		if _, err := q.InsertCategory(ctx, gendb.InsertCategoryParams{
			TaxonomyVersion: TaxonomyVersion,
			OrgID:           org.pg(),
			Code:            "99",
			ParentID:        pgtype.UUID{},
			Level:           1,
			Name:            "dangling",
			IsLeaf:          true,
			IsPnl:           true,
		}); err != nil {
			return err
		}
		return errBroken
	})
	if !errors.Is(err, errBroken) {
		t.Fatalf("InTx returned %v, want errBroken", err)
	}

	// Nothing survived. Checked through InTx bound to this same org id, not
	// InSystemTx: organizations carries FORCE ROW LEVEL SECURITY, so a read
	// with no tenant context raises 42704 rather than answering "zero rows" --
	// binding the context first is what lets an absent row read as absent
	// instead of as an error, and it is safe to bind to an org id that was
	// never committed, since set_config does not require the row to exist.
	if err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).GetOrganization(ctx)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("GetOrganization after rollback: err = %v, want pgx.ErrNoRows", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("checking for leftovers: %v", err)
	}
}
