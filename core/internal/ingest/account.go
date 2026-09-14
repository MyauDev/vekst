package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
)

// errAccountUnresolved is ResolveAccount's answer when st.Account is empty --
// there is nothing to look up and nothing to name a new row after.
var errAccountUnresolved = errors.New("ingest: statement carries no account identifier")

// ResolveAccount finds the accounts row matching st.Account for entityID, or
// creates one if none exists. It is the one check of the twelve (task 2.7)
// that needs a database, so it is not part of ValidateStatement -- it runs
// separately, under the caller's own tenant transaction, and its result is
// merged into a ValidationResult by MergeAccountResolution.
//
// "Unknown and not creatable" (ARCHITECTURE.md §4a.1) fails only when
// st.Account is empty or the insert itself fails; an unknown but
// well-formed identifier is always creatable in the current schema, so this
// resolves far more often than it fails.
//
// mixedCurrency is true when an account already on file for this identifier
// carries a different currency than this statement declares -- the same
// account should not change currency between imports, and a customer who
// downloads a EUR statement for what this system has on file as a BYN
// account almost certainly picked the wrong account, not renamed a real one.
func ResolveAccount(ctx context.Context, tx pgx.Tx, org db.OrgID, entityID uuid.UUID, st *Statement) (accountID uuid.UUID, mixedCurrency bool, err error) {
	if st.Account == "" {
		return uuid.Nil, false, errAccountUnresolved
	}

	q := gendb.New(tx)
	entityPg := pgtype.UUID{Bytes: entityID, Valid: true}
	externalRef := pgtype.Text{String: st.Account, Valid: true}

	existing, err := q.FindAccountByExternalRef(ctx, gendb.FindAccountByExternalRefParams{
		EntityID: entityPg, ExternalRef: externalRef,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		name := st.Holder
		if name == "" {
			name = st.Account
		}
		created, insertErr := q.InsertAccount(ctx, gendb.InsertAccountParams{
			OrgID:       pgtype.UUID{Bytes: org.UUID(), Valid: true},
			EntityID:    entityPg,
			Name:        name,
			Currency:    st.Currency,
			ExternalRef: externalRef,
		})
		if insertErr != nil {
			return uuid.Nil, false, fmt.Errorf("ingest: creating account for %q: %w", st.Account, insertErr)
		}
		return uuid.UUID(created.ID.Bytes), false, nil
	case err != nil:
		return uuid.Nil, false, fmt.Errorf("ingest: resolving account %q: %w", st.Account, err)
	}

	return uuid.UUID(existing.ID.Bytes), existing.Currency != st.Currency, nil
}

// MergeAccountResolution folds ResolveAccount's outcome into a
// ValidationResult already produced by ValidateStatement, recomputing the
// outcome exactly as decideOutcome would have if this check had been part
// of it from the start. Kept as a separate step rather than folded into
// ValidateStatement because that function has no database handle and this
// one does -- see ResolveAccount's own doc comment.
func MergeAccountResolution(result ValidationResult, resolveErr error, mixedCurrency bool) ValidationResult {
	if resolveErr != nil {
		// Line 0: there is no natural line for this fact. The account
		// identifier belongs to the statement, not to any one row.
		result.Errors = append(result.Errors, ValidationError{
			Line: 0, Field: "account", Code: CodeAccountUnresolved,
		})
	}
	if mixedCurrency {
		result.Warnings = append(result.Warnings, ValidationWarning{Code: CodeMixedCurrency})
	}
	return decideOutcome(result.RowCount, result.Errors, result.Warnings, result.BalanceCheckPassed)
}
