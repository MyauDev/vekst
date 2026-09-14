package ingest

import (
	"strings"
	"time"

	"github.com/MyauDev/vekst/core/internal/money"
)

// Correctness error codes. Per row, blocking, and never overridable
// (ARCHITECTURE.md §4a.1, design D1): a batch carrying any of these is
// rejected regardless of what the completeness checks below find.
const (
	CodeDateImplausible       = "date_implausible"
	CodeAmountUnparseable     = "amount_unparseable"
	CodeCurrencyUnknown       = "currency_unknown"
	CodeDebitAndCreditBothSet = "debit_and_credit_both_set"
	CodeReplacementCharacter  = "replacement_character"
	CodeDescriptionMissing    = "description_missing"
	CodeAccountUnresolved     = "account_unresolved"
)

// Completeness warning codes. Per file, and the only ones an override
// (design D1) may ever apply to.
const (
	CodeBalanceMismatch  = "balance_mismatch"
	CodeRowCountMismatch = "row_count_mismatch"
	CodePeriodGap        = "period_gap"
	CodeMixedCurrency    = "mixed_currency"
	CodeDuplicateBankRef = "duplicate_bank_reference"
)

// Outcome is one of import_validations.outcome's three values.
type Outcome string

const (
	OutcomeValid             Outcome = "valid"
	OutcomeValidWithWarnings Outcome = "valid_with_warnings"
	OutcomeRejected          Outcome = "rejected"
)

// ValidationError is one correctness failure, keyed to the line in the
// original file (CLAUDE.md: never by parsed row index).
type ValidationError struct {
	Line  int
	Field string
	Code  string
	Raw   string
}

// BalanceMismatchDetail is CodeBalanceMismatch's structured detail (design
// D5). Minor units throughout -- never a float, like every other amount in
// this system.
type BalanceMismatchDetail struct {
	Opening    int64
	Movements  int64
	Closing    int64
	Difference int64
	Currency   string
}

// ValidationWarning is one completeness finding. Balance is non-nil only
// when Code is CodeBalanceMismatch; the other four completeness codes carry
// no detail today.
type ValidationWarning struct {
	Code    string
	Balance *BalanceMismatchDetail
}

// dateWindow bounds "plausible" (ARCHITECTURE.md §4a.1): no statement a real
// bank issues names a booking date further back than a decade, and "tomorrow"
// admits a value-dated row booked a day ahead without admitting a typo'd
// year.
const dateWindow = 10 * 365 * 24 * time.Hour

// checkRow runs the five per-row correctness checks that vary row to row.
// now is a parameter, never time.Now() read from inside the check, so a
// test can pin "today" instead of racing the calendar (task 2.2). params is
// add-import-profiles' DateFormat override; nil (or an empty DateFormat)
// tries this package's own known layouts exactly as before params existed.
func checkRow(r Row, currency string, now time.Time, params *Parameters) []ValidationError {
	var errs []ValidationError
	add := func(field, code, raw string) {
		errs = append(errs, ValidationError{Line: r.LineNo, Field: field, Code: code, Raw: raw})
	}

	parseDate := ParseDate
	if params != nil && params.DateFormat != "" {
		layout := params.DateFormat
		parseDate = func(raw string) (time.Time, error) {
			return time.ParseInLocation(layout, strings.TrimSpace(raw), time.UTC)
		}
	}
	if t, err := parseDate(r.BookedOn); err != nil {
		add("booked_on", CodeDateImplausible, r.BookedOn)
	} else {
		earliest := now.Add(-dateWindow)
		latest := now.Add(24 * time.Hour)
		if t.Before(earliest) || t.After(latest) {
			add("booked_on", CodeDateImplausible, r.BookedOn)
		}
	}

	if _, err := ParseAmount(currency, r.DebitRaw); err != nil {
		add("debit", CodeAmountUnparseable, r.DebitRaw)
	}
	if _, err := ParseAmount(currency, r.CreditRaw); err != nil {
		add("credit", CodeAmountUnparseable, r.CreditRaw)
	}

	// Both non-zero, not both populated -- change 2.2 design D5. Every real
	// Priorbank row populates both columns, one of them "0,00"; only a row
	// where neither side is zero is actually malformed.
	if r.Debit.MinorUnits != 0 && r.Credit.MinorUnits != 0 {
		add("debit", CodeDebitAndCreditBothSet, r.DebitRaw+" / "+r.CreditRaw)
	}

	// A slice of pairs, not a map: this package's parsers guarantee the same
	// bytes always produce the same Statement, and a map's iteration order
	// would make that true of the parsed values but not of the order errors
	// land in this list.
	replacementFields := []struct{ field, value string }{
		{"booked_on", r.BookedOn},
		{"document_no", r.DocumentNo},
		{"counterparty_name", r.CounterpartyName},
		{"description", r.Description},
	}
	for _, f := range replacementFields {
		if ContainsReplacementChar(f.value) {
			add(f.field, CodeReplacementCharacter, f.value)
		}
	}

	if strings.TrimSpace(r.Description) == "" {
		add("description", CodeDescriptionMissing, r.Description)
	}

	return errs
}

// checkCurrency is evaluated once per statement, not once per row: every row
// shares one currency, and reporting the same failure once per row in a
// multi-thousand-row file is the "report larger than the file" risk the
// design's own risk table names. A statement-level failure still blocks
// every row -- see validate.go's caller, which is why ARCHITECTURE.md lists
// it under "per row" even though it is evaluated once.
func checkCurrency(currency string) *ValidationError {
	if _, ok := money.Exponent(currency); !ok {
		return &ValidationError{Line: 0, Field: "currency", Code: CodeCurrencyUnknown, Raw: currency}
	}
	return nil
}
