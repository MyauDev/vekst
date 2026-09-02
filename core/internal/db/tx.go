package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// InTx is the only place in the codebase that begins a transaction. Every
// caller -- an HTTP handler, a Connect RPC handler, a River worker -- goes
// through here. A nil error from fn commits; any other error rolls back and
// is returned unwrapped, so callers can still errors.Is against it.
//
// Change 1.1 adds `SET LOCAL app.org_id` here, and nowhere else. River's own
// tables carry no org_id and no row-level-security policy (design D4), so a
// worker's fn must set app.org_id from its own job arguments -- a job
// payload is untrusted input for tenancy purposes, never ambient state, the
// same way a handler sets it from the authenticated session and not from a
// global. See design D2.
func (d *DB) InTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
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
