package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

// adoptIndustryTemplate copies every category_templates row for
// taxonomyVersion into org's own categories, as scope = 'org' rows, inside
// tx -- CreateOrganization's transaction, so an organisation with no
// taxonomy beyond the shared level-1 and level-2 nodes is not a state that
// survives a failure partway through (design §2.2 for change 5.3,
// connect-app-end-to-end).
//
// It lives here rather than in core/internal/tenancy, where the design that
// asked for it first put it: CreateOrganization has to call it inside its
// own transaction or none of it, and core/internal/tenancy already needs to
// import core/internal/db for OrgID, DB and MembershipsForUser, so db
// importing tenancy back would be a cycle. tenancy still owns organisation
// creation as a product action -- validation, the already_a_member refusal,
// the call to CreateOrganization -- this function is the one piece that has
// to live beside the transaction it runs in.
//
// IndustryTemplate's own ORDER BY (level, code) is load-bearing: a row's
// parent resolves to this organisation's own copy first, and only then to
// the shared category with the same code -- 030101 hangs under global 0301,
// but 0401010101 hangs under this organisation's own 04010101, which must
// already have been inserted by an earlier iteration of this same loop for
// that to work. A row whose parent resolves to neither fails the whole
// transaction: an organisation with a dangling leaf is not a state worth
// being able to represent.
func adoptIndustryTemplate(ctx context.Context, tx pgx.Tx, org OrgID, taxonomyVersion string) error {
	q := gendb.New(tx)

	rows, err := q.IndustryTemplate(ctx, taxonomyVersion)
	if err != nil {
		return fmt.Errorf("db: listing industry template: %w", err)
	}

	shared, err := q.EffectiveTaxonomy(ctx, taxonomyVersion)
	if err != nil {
		return fmt.Errorf("db: listing shared taxonomy: %w", err)
	}
	globalByCode := make(map[string]pgtype.UUID, len(shared))
	for _, c := range shared {
		if !c.OrgID.Valid { // the shared rows only; this organisation owns none yet
			globalByCode[c.Code] = c.ID
		}
	}

	orgByCode := make(map[string]pgtype.UUID, len(rows))
	for _, r := range rows {
		var parentID pgtype.UUID
		if r.ParentCode.Valid {
			code := r.ParentCode.String
			if id, ok := orgByCode[code]; ok {
				parentID = id
			} else if id, ok := globalByCode[code]; ok {
				parentID = id
			} else {
				return fmt.Errorf("db: industry template row %q names parent %q, which resolves to neither this organisation's own rows nor a shared category", r.Code, code)
			}
		}

		insertedID, err := q.InsertCategory(ctx, gendb.InsertCategoryParams{
			TaxonomyVersion: taxonomyVersion,
			OrgID:           org.pg(),
			Code:            r.Code,
			ParentID:        parentID,
			Level:           r.Level,
			Name:            r.Name,
			IsLeaf:          r.IsLeaf,
			IsPnl:           r.IsPnl,
		})
		if err != nil {
			return fmt.Errorf("db: adopting category %q: %w", r.Code, err)
		}
		orgByCode[r.Code] = insertedID
	}
	return nil
}
