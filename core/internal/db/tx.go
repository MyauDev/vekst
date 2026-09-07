package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// InSystemTx begins a transaction that sets no tenant context. Every caller
// -- an HTTP handler, a Connect RPC handler, a River worker -- reaches the
// database through here or through the tenant-aware InTx that change 1.1
// adds beside it. A nil error from fn commits; any other error rolls back and
// is returned unwrapped, so callers can still errors.Is against it.
//
// It is named for what it is so that reaching for it is a visible choice. Its
// legitimate callers are few and listed: the readiness check, migration
// status, and core/internal/identity, which resolves a session cookie before
// any organisation is known and so cannot supply one. Change 1.1 counts these
// call sites against a committed expected number, so a new one is a diff
// rather than a habit.
//
// Change 1.1 adds `SET LOCAL app.org_id` in InTx, not here -- this entry
// point is precisely the one that never sets it. River's own tables carry no
// org_id and no row-level-security policy (design D4), so a worker's fn takes
// its tenant identifier from its own job arguments: a job payload is
// untrusted input for tenancy purposes, never ambient state, the same way a
// handler takes it from the authenticated session and not from a global. See
// design D2 and add-identity design D4.
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
