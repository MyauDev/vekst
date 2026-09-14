package ingest

import (
	"strconv"
	"time"
)

// ValidationResult is what running every check produces, before it becomes
// import_validations' outcome, counts and report_jsonb.
type ValidationResult struct {
	Outcome  Outcome
	RowCount int
	Errors   []ValidationError
	Warnings []ValidationWarning

	// BalanceCheckPassed is nil when the file declared no balances to check
	// (Statement.HasDeclaredBalance is false) -- a fact distinct from "did
	// not reconcile", which import_validations' own tri-state column exists
	// to keep distinct (design D2).
	BalanceCheckPassed *bool
}

// ValidateStatement runs every correctness and completeness check against
// st, in that order (task 5.1), and decides the outcome. It takes no
// database handle: "account identifier resolves" is the one check that
// needs one, and it is layered on afterward by the caller (ingest.Service),
// which merges its own error or warning into the result before the outcome
// -- see MergeAccountResolution. Keeping this function pure is what makes it
// unit-testable against a Statement built by hand, the same shape as this
// package's parsers.
func ValidateStatement(st *Statement, now time.Time) ValidationResult {
	return ValidateStatementWithParams(st, now, nil)
}

// ValidateStatementWithParams is ValidateStatement with add-import-profiles'
// DateFormat override applied to the date-plausible check (task 6.2/6.3): a
// nil params, or an empty DateFormat within one, tries this package's own
// known layouts exactly as ValidateStatement always has. A profile that
// names one tries only that layout -- design D1's "a set field replaces
// detection for that field," not an addition to the list.
func ValidateStatementWithParams(st *Statement, now time.Time, params *Parameters) ValidationResult {
	var errs []ValidationError
	var warns []ValidationWarning

	for _, r := range st.Rows {
		errs = append(errs, checkRow(r, st.Currency, now, params)...)
	}
	if e := checkCurrency(st.Currency); e != nil {
		errs = append(errs, *e)
	}

	var balancePassed *bool
	if st.HasDeclaredBalance {
		ok, diff, err := st.BalanceCheck()
		if err == nil {
			passed := ok
			balancePassed = &passed
			if !ok {
				var movements int64
				for _, r := range st.Rows {
					movements += r.Credit.MinorUnits - r.Debit.MinorUnits
				}
				warns = append(warns, ValidationWarning{
					Code: CodeBalanceMismatch,
					Balance: &BalanceMismatchDetail{
						Opening:    st.Opening.MinorUnits,
						Movements:  movements,
						Closing:    st.Closing.MinorUnits,
						Difference: diff.MinorUnits,
						Currency:   st.Currency,
					},
				})
			}
		}
	}

	if w := checkPeriodContinuity(st); w != nil {
		warns = append(warns, *w)
	}
	if w := checkDuplicateBankReference(st); w != nil {
		warns = append(warns, *w)
	}
	if w := checkRowCount(st); w != nil {
		warns = append(warns, *w)
	}

	return decideOutcome(len(st.Rows), errs, warns, balancePassed)
}

// checkPeriodContinuity is a simplified reading of "the declared period is
// covered continuously" (ARCHITECTURE.md §4a.1): every row falls within the
// bank's own declared [PeriodFrom, PeriodTo], both inclusive. This catches a
// file that starts mid-period or carries a row the bank itself would not
// have included -- it does not detect a gap *inside* an otherwise
// in-bounds range, which no document specifies an algorithm for and which a
// business with no weekend transactions would trip on every week if it
// were interpreted as "every day has a row".
func checkPeriodContinuity(st *Statement) *ValidationWarning {
	if st.PeriodFrom == "" || st.PeriodTo == "" {
		return nil // nothing declared, nothing to check -- same shape as the balance check.
	}
	from, err := ParseDate(st.PeriodFrom)
	if err != nil {
		return nil
	}
	to, err := ParseDate(st.PeriodTo)
	if err != nil {
		return nil
	}
	for _, r := range st.Rows {
		booked, err := ParseDate(r.BookedOn)
		if err != nil {
			continue // already a correctness error (CodeDateImplausible); not this check's to repeat.
		}
		if booked.Before(from) || booked.After(to) {
			return &ValidationWarning{Code: CodePeriodGap}
		}
	}
	return nil
}

// checkDuplicateBankReference catches a double-appended export: the same
// posting, byte for byte, twice in one file.
//
// DocumentNo ("N док.") alone is not that key -- verified against a real
// redacted Priorbank export, where it repeats across genuinely distinct
// transactions on different dates (a bank's own "N док." is closer to a
// batch or document-type number than a per-posting reference). The
// composite below -- date, document number, counterparty account, both
// amounts -- is what a double-appended row would actually match on exactly;
// two unrelated transactions sharing a document number but differing in
// date or amount do not collide.
func checkDuplicateBankReference(st *Statement) *ValidationWarning {
	seen := make(map[string]bool, len(st.Rows))
	for _, r := range st.Rows {
		key := r.BookedOn + "|" + r.DocumentNo + "|" + r.CounterpartyAccount + "|" +
			strconv.FormatInt(r.Debit.MinorUnits, 10) + "|" + strconv.FormatInt(r.Credit.MinorUnits, 10)
		if seen[key] {
			return &ValidationWarning{Code: CodeDuplicateBankRef}
		}
		seen[key] = true
	}
	return nil
}

// checkRowCount compares the file's own declared count against what was
// actually parsed. Priorbank declares none -- st.DeclaredRowCount is nil for
// every statement this package's own parser produces today, and this check
// is a no-op for it -- but a 1C ledger export often does, and this is where
// a future ledger parser's declared count gets checked against reality.
func checkRowCount(st *Statement) *ValidationWarning {
	if st.DeclaredRowCount == nil {
		return nil
	}
	if len(st.Rows) != *st.DeclaredRowCount {
		return &ValidationWarning{Code: CodeRowCountMismatch}
	}
	return nil
}

// decideOutcome is design's own rule, stated as code: any correctness error
// rejects the batch regardless of what completeness found: the
// import_validations_counts_match_outcome constraint requires exactly this
// shape, and getting it right in Go is what keeps the write from being
// refused by the database it is about to reach.
func decideOutcome(rowCount int, errs []ValidationError, warns []ValidationWarning, balancePassed *bool) ValidationResult {
	outcome := OutcomeValid
	switch {
	case len(errs) > 0:
		outcome = OutcomeRejected
	case len(warns) > 0:
		outcome = OutcomeValidWithWarnings
	}
	return ValidationResult{
		Outcome:            outcome,
		RowCount:           rowCount,
		Errors:             errs,
		Warnings:           warns,
		BalanceCheckPassed: balancePassed,
	}
}
