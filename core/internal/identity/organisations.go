package identity

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

// Entity is one entity within an organisation, for GetCurrentUser.
type Entity struct {
	ID   uuid.UUID
	Name string
}

// Organisation is one organisation the caller belongs to, for GetCurrentUser.
// An empty list from Organisations is the first-run signal: a person who has
// just signed in for the first time has not failed at anything.
type Organisation struct {
	ID           uuid.UUID
	Name         string
	BaseCurrency string
	Role         string
	Entities     []Entity
}

// Organisations lists every organisation userID belongs to, each with its
// entities.
//
// Two steps, the same shape this package's own doc comment already asks for
// when reading users once organisations exist: db.MembershipsForUser answers
// "which ones" the way OrgIDForSession answers "is it this one", and then
// each organisation and its entities are fetched by the key that answer
// gave -- under that organisation's own row-level security, one InTx per
// membership, never a query that reaches across more than one tenant at
// once.
func (s *Service) Organisations(ctx context.Context, userID uuid.UUID) ([]Organisation, error) {
	memberships, err := s.database.MembershipsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("identity: listing memberships: %w", err)
	}

	orgs := make([]Organisation, 0, len(memberships))
	for _, m := range memberships {
		var org Organisation
		err := s.database.InTx(ctx, m.Org, func(ctx context.Context, tx pgx.Tx) error {
			q := gendb.New(tx)

			o, err := q.GetOrganization(ctx)
			if err != nil {
				return fmt.Errorf("get organization: %w", err)
			}
			ents, err := q.ListEntities(ctx)
			if err != nil {
				return fmt.Errorf("list entities: %w", err)
			}

			org = Organisation{
				ID:           m.Org.UUID(),
				Name:         o.Name,
				BaseCurrency: o.BaseCurrency,
				Role:         string(m.Role),
				Entities:     make([]Entity, 0, len(ents)),
			}
			for _, e := range ents {
				org.Entities = append(org.Entities, Entity{ID: uuid.UUID(e.ID.Bytes), Name: e.Name})
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("identity: reading organisation %s: %w", m.Org, err)
		}
		orgs = append(orgs, org)
	}
	return orgs, nil
}
