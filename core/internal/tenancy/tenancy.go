// Package tenancy owns creating an organisation as a product action: turning
// a signed-in caller with no membership into one with an organisation, an
// entity and an owner role.
//
// The mechanics of the transaction itself -- minting the identifier, writing
// the three rows, adopting the industry template -- live in
// core/internal/db, because CreateOrganization has to do all of it inside
// one transaction or none of it, and this package already has to import
// core/internal/db for OrgID and DB. What this package owns is everything
// around that transaction: validating the request, refusing a caller who
// already belongs to somewhere, and translating the result.
package tenancy

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/money"
)

// Error codes. The backend returns codes, never sentences -- translation is
// the client's (CLAUDE.md, Conventions).
const (
	CodeNameRequired        = "org_name_required"
	CodeEntityNameRequired  = "org_entity_name_required"
	CodeUnsupportedCountry  = "org_unsupported_country"
	CodeUnsupportedCurrency = "org_unsupported_currency"
	CodeAlreadyMember       = "org_already_a_member"
)

// Err is a failure with a code a client can translate.
type Err struct {
	Code string
	err  error
}

func (e *Err) Error() string { return e.Code }
func (e *Err) Unwrap() error { return e.err }

func fail(code string, err error) *Err { return &Err{Code: code, err: err} }

// New is what a caller supplies to create an organisation.
type New struct {
	Name         string
	Country      string // ISO-3166-1 alpha-2
	BaseCurrency string // ISO-4217
	EntityName   string // the single entity every organisation has in the Demo
}

// Created is what creating one returns.
type Created struct {
	OrgID    uuid.UUID
	EntityID uuid.UUID
}

// supportedCountries is the Demo's allowlist, matching the three pilot
// markets the STATUS report names: Belarus, Kazakhstan, Poland. There is no
// countries table to lean on any more than there was a currencies table
// (design §2.2's evidence for currencies applies the same way here), so this
// validates in Go, the same shape as the currency check below.
var supportedCountries = map[string]bool{
	"BY": true,
	"KZ": true,
	"PL": true,
}

func validCountry(code string) bool { return supportedCountries[code] }

// Service creates organisations.
type Service struct {
	db *db.DB
}

// NewService wraps database for the tenancy operations it is responsible
// for. Named NewService rather than New because New is already the request
// type above -- tenancy.New(...) reads as constructing a request, not a
// service, and the two should not collide.
func NewService(database *db.DB) *Service {
	return &Service{db: database}
}

// Create validates in, refuses a caller who already belongs to an
// organisation, and creates one -- with its first entity, the caller's owner
// membership, and the adopted industry template, all inside
// db.CreateOrganization's single transaction.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, in New) (Created, error) {
	if in.Name == "" {
		return Created{}, fail(CodeNameRequired, errors.New("tenancy: name is required"))
	}
	if in.EntityName == "" {
		return Created{}, fail(CodeEntityNameRequired, errors.New("tenancy: entity_name is required"))
	}
	if !validCountry(in.Country) {
		return Created{}, fail(CodeUnsupportedCountry, fmt.Errorf("tenancy: %q is not a supported country", in.Country))
	}
	// core/internal/money's exponent map, not organizations.base_currency's own
	// CHECK: the column accepts any three uppercase letters (design §2.2), so
	// the refusal that matters happens here, in Go, before any row is written.
	if _, ok := money.Exponent(in.BaseCurrency); !ok {
		return Created{}, fail(CodeUnsupportedCurrency, fmt.Errorf("tenancy: %q is not a supported currency", in.BaseCurrency))
	}

	// The already_a_member refusal, checked before CreateOrganization opens
	// its own transaction: a caller who already belongs somewhere is refused
	// without writing anything, rather than writing and rolling back.
	memberships, err := s.db.MembershipsForUser(ctx, userID)
	if err != nil {
		return Created{}, fmt.Errorf("tenancy: checking existing memberships: %w", err)
	}
	if len(memberships) > 0 {
		return Created{}, fail(CodeAlreadyMember, errors.New("tenancy: caller already belongs to an organisation"))
	}

	org, err := s.db.CreateOrganization(ctx, db.NewOrganization{
		Name:         in.Name,
		Country:      in.Country,
		BaseCurrency: in.BaseCurrency,
		EntityName:   in.EntityName,
		CreatorID:    userID,
	})
	if err != nil {
		return Created{}, fmt.Errorf("tenancy: creating organisation: %w", err)
	}

	// The entity id is not returned by CreateOrganization -- it commits the
	// transaction and returns only the OrgID, the same way it always has for
	// its existing test callers. Reading it back is one InTx under the
	// organisation that now exists, rather than widening CreateOrganization's
	// return shape for the one caller that needs the entity id too.
	entities, err := s.entityFor(ctx, org)
	if err != nil {
		return Created{}, fmt.Errorf("tenancy: reading back the new entity: %w", err)
	}

	return Created{OrgID: org.UUID(), EntityID: entities}, nil
}

// entityFor reads back the single entity CreateOrganization just wrote. The
// Demo gives every organisation exactly one, so the first row is the only
// one; a later change adding a second entity is also the change that gives
// this a reason to return more than one.
func (s *Service) entityFor(ctx context.Context, org db.OrgID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		entities, err := gendb.New(tx).ListEntities(ctx)
		if err != nil {
			return err
		}
		if len(entities) == 0 {
			return errors.New("tenancy: organisation has no entity")
		}
		id = uuid.UUID(entities[0].ID.Bytes)
		return nil
	})
	return id, err
}
