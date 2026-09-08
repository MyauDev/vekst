package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// InSystemTx begins a transaction that sets no tenant context. Every caller
// -- an HTTP handler, a Connect RPC handler, a River worker -- reaches the
// database through here or through the tenant-aware InTx below. A nil error
// from fn commits; any other error rolls back and is returned unwrapped, so
// callers can still errors.Is against it.
//
// It is named for what it is so that reaching for it is a visible choice, and
// its legitimate callers are few, listed, and counted. There are exactly
// three kinds:
//
//   - The readiness check and migration status, which read goose_db_version.
//     That table is infrastructure, allowlisted in
//     deploy/db/rls-exempt-tables.txt, and belongs to no tenant.
//   - core/internal/identity, which resolves a session cookie. It runs before
//     any organisation is known and reads only the four global identity
//     tables, which have no policy for the same reason.
//   - OrgIDForSession, which calls orgs_for_user. This is the read that
//     turns "which organisations may this user act for?" into an answer, so
//     by definition it cannot already have one.
//
// scripts/check-db-entry-point.sh counts these call sites against a committed
// number, so a fourth kind is a diff rather than a habit. The list is short
// because everything else has a tenant, and everything with a tenant goes
// through InTx.
//
// What must never happen here is a read of a tenant table. There is no
// silent-empty failure mode to worry about -- app_current_org() raises rather
// than returning NULL (migration 00004), so such a read fails loudly -- but
// it fails at run time, and the committed count is what catches it in review
// instead.
func (d *DB) InSystemTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin: %w", err)
	}

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return fmt.Errorf("db: rollback after %w: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}

// InTx begins a transaction bound to one organisation and sets the tenant
// context as its first statement, transaction-locally, here and nowhere else.
// Every caller that touches tenant data reaches the database through this --
// a Connect RPC handler with an organisation resolved from the session, a
// River worker with one taken from its own job arguments. A nil error from fn
// commits; any other error rolls back and is returned unwrapped.
//
// The organisation is a required argument of a type no other package can
// construct (design D2 and D7). Required means it cannot be forgotten:
// omitting it does not compile, rather than reading tenant zero at run time
// the way a value carried in context.Context could. Unconstructable means it
// cannot be invented: the OrgID came from one of db's named constructors, so
// row-level security is isolating the transaction to an organisation the
// caller was established to act for, not merely to one it named.
//
// set_config's third argument is true -- transaction-local. The setting is
// reverted when this transaction ends, so the next user of this pooled
// connection cannot inherit it. It is passed as a bound parameter and never
// interpolated into a SET statement: SET does not take parameters, so the
// alternative is string concatenation of a tenant identifier into SQL text,
// and scripts/check-db-entry-point.sh fails a build that adds one.
func (d *DB) InTx(ctx context.Context, org OrgID, fn func(context.Context, pgx.Tx) error) error {
	// Checked before opening a transaction so a forgotten assignment costs
	// nothing and names its own fix. The zero value would otherwise reach
	// Postgres as the nil UUID, match no policy, and surface as an empty
	// result or a foreign key failure some statements later.
	if org.IsZero() {
		return errors.New("db: InTx called with a zero OrgID; obtain one from " +
			"OrgIDForSession, OrgIDFromJobArgs or OrgIDForNewOrg")
	}

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin: %w", err)
	}

	if _, err := tx.Exec(ctx, `SELECT set_config('app.org_id', $1, true)`, org.String()); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return fmt.Errorf("db: rollback after setting tenant context: %v (%w)", rbErr, err)
		}
		return fmt.Errorf("db: setting tenant context: %w", err)
	}

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return fmt.Errorf("db: rollback after %w: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}
