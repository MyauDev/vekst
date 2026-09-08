package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// OrgID is a tenant identifier that has been *established*, not asserted.
//
// The field is unexported, so no package outside db can build one from a raw
// UUID. That is the whole mechanism, and it exists because row-level security
// cannot supply this guarantee itself. RLS is containment, not authorization:
// it guarantees "a transaction bound to org X touches only X's rows", and it
// has no way to know whether the caller was ever entitled to bind to X. With
// an ordinary `type OrgID uuid.UUID`, this would compile from any package:
//
//	org := db.OrgID(uuid.MustParse(req.Msg.GetOrgId()))   // straight off the wire
//
// and the database would then isolate the transaction perfectly, to the
// organisation an attacker named. InTx taking a required argument stops the
// organisation being *forgotten*; the unexported field is what stops it being
// *invented*.
//
// Every value therefore came through one of the three constructors below, and
// those are the complete list of ways a request can come to act for a tenant:
// an authenticated session whose membership was resolved, a background job's
// own arguments, and the creation of a new organisation. Two of the three have
// their call sites counted by scripts/check-db-entry-point.sh, so adding one
// is a visible diff rather than a habit. See design D7.
type OrgID struct{ v uuid.UUID }

// IsZero reports whether this is the zero value -- a forgotten assignment
// rather than a resolved organisation. The zero value is inert on its own (the
// nil UUID matches no row and satisfies no policy, so it fails closed), but
// InTx rejects it explicitly anyway, naming the constructor to use.
func (o OrgID) IsZero() bool { return o.v == uuid.Nil }

// UUID returns the identifier for use as a query parameter. It does not widen
// the type: a caller who has an OrgID already holds the authority this returns,
// and nothing accepts a bare uuid.UUID as a tenant context.
func (o OrgID) UUID() uuid.UUID { return o.v }

// String makes an OrgID safe to log. It is an identifier, not a secret.
func (o OrgID) String() string { return o.v.String() }

// pg renders the identifier as sqlc's parameter type.
func (o OrgID) pg() pgtype.UUID { return pgtype.UUID{Bytes: o.v, Valid: true} }

// Role is a membership role. It is stored, returned, and -- for now --
// checked by nothing: role enforcement is Product's, per this change's
// non-goals. It is returned by OrgIDForSession rather than looked up again at
// enforcement time, because a second lookup later is how a stale-permissions
// bug is built.
type Role string

// TenantJobArgs is embedded by every River job argument struct that needs a
// tenant. It is declared here rather than in core/internal/jobs because jobs
// already depends on db and the dependency runs one way -- workers embed this
// in their own argument structs rather than db learning anything about River.
type TenantJobArgs struct {
	OrgID uuid.UUID `json:"org_id"`
}

// ErrNotAMember is returned when a caller asks to act for an organisation
// they do not belong to.
//
// It is deliberately the same error whether the organisation does not exist or
// the user is simply not a member. Distinguishing them turns this call into a
// membership oracle -- ask for a guessed identifier, and the difference in the
// error tells you whether it names a real organisation. That is the same
// disclosure the composite foreign keys close at the schema level (design D1),
// and it would be pointless to close it there and reopen it here.
var ErrNotAMember = errors.New("db: not a member of that organisation")

// OrgIDForSession resolves the organisation an authenticated caller asked to
// act for, and is the door that is *supposed* to be used -- it is the only one
// whose call sites are not counted.
//
// It is a method on *DB because it has to reach the database before any
// organisation is known: memberships has a policy, and app_current_org() would
// raise inside a tenant transaction that does not exist yet. So it opens
// InSystemTx and calls orgs_for_user, the single SECURITY DEFINER function,
// which reaches memberships through a policy scoped to a NOLOGIN role that
// vekst_app cannot assume (design D3).
//
// The query is written by hand rather than generated: sqlc cannot resolve the
// output columns of a RETURNS TABLE function, and the shapes that satisfy it
// would widen the function's signature -- see the note in query/tenancy.sql.
//
// Nothing is cached. It is one index scan on memberships (user_id), and
// caching it is how a revoked user keeps their access until their session
// expires.
func (d *DB) OrgIDForSession(ctx context.Context, userID, requested uuid.UUID) (OrgID, Role, error) {
	if requested == uuid.Nil {
		return OrgID{}, "", ErrNotAMember
	}

	var role Role
	found := false
	err := d.InSystemTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT org_id, role FROM orgs_for_user($1)`,
			pgtype.UUID{Bytes: userID, Valid: true})
		if err != nil {
			return fmt.Errorf("db: orgs_for_user: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var org pgtype.UUID
			var r string
			if err := rows.Scan(&org, &r); err != nil {
				return fmt.Errorf("db: scanning membership: %w", err)
			}
			if uuid.UUID(org.Bytes) == requested {
				role, found = Role(r), true
			}
		}
		return rows.Err()
	})
	if err != nil {
		return OrgID{}, "", err
	}
	if !found {
		return OrgID{}, "", ErrNotAMember
	}
	return OrgID{v: requested}, role, nil
}

// OrgIDFromJobArgs is the worker door (design D5). River's own tables carry no
// tenant column and no policy, so a job's arguments are the only place its
// tenant can come from -- and they are treated as input, never as ambient
// state, the same way a handler takes its tenant from the authenticated
// session and not from a global.
//
// It does not verify membership: a job is enqueued by code that already held
// an OrgID, so the authority was established when the job was created rather
// than when it runs. That is why its call sites are counted.
func OrgIDFromJobArgs(args TenantJobArgs) (OrgID, error) {
	if args.OrgID == uuid.Nil {
		return OrgID{}, errors.New("db: job arguments carry no organisation; a worker cannot run without a tenant context")
	}
	return OrgID{v: args.OrgID}, nil
}

// OrgIDForNewOrg mints the identifier for an organisation that does not exist
// yet, for the one insert that creates it (design D4).
//
// The identifier is generated here, before the row exists, precisely because
// the policy on organizations is `id = app_current_org()`: the insert needs
// the tenant context to already name the row being inserted. That is not an
// obstacle to work around -- it is what lets a tenant be created under its own
// policy, with no privileged path.
func OrgIDForNewOrg() OrgID {
	return OrgID{v: uuid.New()}
}
