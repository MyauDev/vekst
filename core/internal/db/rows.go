package db

import (
	"errors"
	"fmt"
)

// ErrNoRowsAffected is returned by ExactlyOneRow when a write intended to
// touch a single row touched none.
//
// It is a distinct error rather than a nil return because row-level security
// filters UPDATE and DELETE *silently*. A statement whose target row the
// current tenant policy does not admit affects zero rows and raises nothing:
// the transaction commits, the caller sees no error, and the row is unchanged.
// INSERT is the loud case -- a WITH CHECK violation raises 42501 -- so update
// and delete are the two that need this.
//
// For this product the quiet failure is the dangerous one. "The correction was
// saved" when it was not is the same class of bug as a report showing another
// company's figures: the system reports success and the number is wrong. It
// bears directly on the append-only rule in CLAUDE.md, too -- superseding a
// classification is an UPDATE of the superseded row, and a silent no-op there
// leaves two live classifications for one transaction.
//
// A zero count does not distinguish "the policy filtered it" from "no such
// row", and deliberately so: that is the same non-disclosure the composite
// foreign keys provide (design D1). The caller learns the write did not
// happen, not whose row it was.
var ErrNoRowsAffected = errors.New("db: write affected no rows")

// ExactlyOneRow adapts a sqlc :execrows call into an error-returning one,
// composing directly with the generated signature:
//
//	err := db.ExactlyOneRow(q.UpdateEntityName(ctx, params))
//
// Every generated write whose intent is "exactly one row" goes through here.
// A statement that legitimately affects several rows is a :exec or :many and
// does not belong in this wrapper; one that affects more than one row when it
// meant to affect one is a bug in the WHERE clause, and is reported as such.
func ExactlyOneRow(affected int64, err error) error {
	if err != nil {
		return err
	}
	switch {
	case affected == 0:
		return ErrNoRowsAffected
	case affected > 1:
		// Not a tenancy failure -- the policy can only ever reduce the count.
		// This means the statement's own predicate matched more rows than the
		// caller believed existed, which for a single-row write is a defect
		// worth surfacing rather than rounding down to success.
		return fmt.Errorf("db: write affected %d rows, want exactly 1", affected)
	}
	return nil
}
