package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

// NewOrganization is what a caller supplies to create a tenant. The identifier
// is not among these fields: it is minted by OrgIDForNewOrg inside
// CreateOrganization, because the identifier and the tenant context have to be
// the same value and nothing outside db can produce one.
type NewOrganization struct {
	Name         string
	Country      string // ISO-3166-1 alpha-2
	BaseCurrency string // ISO-4217
	EntityName   string // the single entity every organisation has in the Demo
	CreatorID    uuid.UUID
}

// CreateOrganization creates an organisation, its first entity and the
// creator's owner membership, in one transaction, under that organisation's
// own row-level-security policy (design D4).
//
// There is no privileged path here and no exception to enable. The policy on
// organizations is `id = app_current_org()`, so the insert needs the tenant
// context to already name the row being inserted -- which is exactly what
// minting the identifier first arranges. WITH CHECK passes because the two
// identifiers are the same value, and the entity and membership rows that
// follow are stamped with the same organisation, inside the same transaction,
// so they satisfy the same policies for the same reason.
//
// If any of the three fails, none of them lands: an organisation with no
// entity, or with nobody able to reach it, is not a state worth being able to
// represent.
func (d *DB) CreateOrganization(ctx context.Context, in NewOrganization) (OrgID, error) {
	if in.CreatorID == uuid.Nil {
		return OrgID{}, fmt.Errorf("db: creating an organisation needs a creator to own it")
	}

	org := OrgIDForNewOrg()

	err := d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		if _, err := q.InsertOrganization(ctx, gendb.InsertOrganizationParams{
			ID:           org.pg(),
			Name:         in.Name,
			Country:      in.Country,
			BaseCurrency: in.BaseCurrency,
		}); err != nil {
			return fmt.Errorf("db: inserting organisation: %w", err)
		}

		// entity_id is populated from the first migration even though the Demo
		// gives each organisation exactly one entity, so a holding customer
		// needs no migration of existing data -- only new rows.
		if _, err := q.InsertEntity(ctx, gendb.InsertEntityParams{
			OrgID: org.pg(),
			Name:  in.EntityName,
		}); err != nil {
			return fmt.Errorf("db: inserting entity: %w", err)
		}

		if _, err := q.InsertMembership(ctx, gendb.InsertMembershipParams{
			OrgID:  org.pg(),
			UserID: pgtype.UUID{Bytes: in.CreatorID, Valid: true},
			Role:   "owner",
		}); err != nil {
			return fmt.Errorf("db: inserting owner membership: %w", err)
		}
		return nil
	})
	if err != nil {
		return OrgID{}, err
	}
	return org, nil
}
