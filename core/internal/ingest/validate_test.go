package ingest

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/MyauDev/vekst/core/internal/money"
)

func mustMoney(t *testing.T, currency string, minor int64) money.Money {
	t.Helper()
	m, err := money.New(currency, minor)
	if err != nil {
		t.Fatalf("money.New(%s, %d): %v", currency, minor, err)
	}
	return m
}

// baseRow is a row that passes every correctness check, so each check's
// failing fixture only needs to break the one thing it tests (task 6.2).
func baseRow(t *testing.T, currency string) Row {
	t.Helper()
	return Row{
		LineNo:      10,
		BookedOn:    "15.06.2026",
		DocumentNo:  "1",
		Description: "payment",
		Debit:       mustMoney(t, currency, 0),
		Credit:      mustMoney(t, currency, 5000),
		DebitRaw:    "0,00",
		CreditRaw:   "50,00",
	}
}

var fixedNow = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

// --- task 6.1: the check table matches ARCHITECTURE.md §4a.1 -------------

// allCorrectnessCodes and allCompletenessCodes are written out by hand from
// the corrected table in ARCHITECTURE.md §4a.1, independently of the
// constants above -- so a code added to code without the document, or a row
// removed from the document without the code, shows up as a length or
// membership mismatch rather than a test confirming itself.
var allCorrectnessCodes = []string{
	CodeDateImplausible, CodeAmountUnparseable, CodeCurrencyUnknown,
	CodeDebitAndCreditBothSet, CodeReplacementCharacter, CodeDescriptionMissing,
	CodeAccountUnresolved,
}

var allCompletenessCodes = []string{
	CodeBalanceMismatch, CodeRowCountMismatch, CodePeriodGap,
	CodeMixedCurrency, CodeDuplicateBankRef,
}

func TestCheckTableHasTwelveEntries(t *testing.T) {
	if got := len(allCorrectnessCodes); got != 7 {
		t.Errorf("%d correctness checks, want 7 (ARCHITECTURE.md §4a.1)", got)
	}
	if got := len(allCompletenessCodes); got != 5 {
		t.Errorf("%d completeness checks, want 5 (ARCHITECTURE.md §4a.1)", got)
	}
	seen := map[string]bool{}
	for _, c := range append(append([]string{}, allCorrectnessCodes...), allCompletenessCodes...) {
		if seen[c] {
			t.Errorf("code %q listed twice", c)
		}
		seen[c] = true
	}
}

// --- task 6.2: twelve pairs, a clean fixture and a dirty one -------------

func TestDateImplausible(t *testing.T) {
	pass := baseRow(t, "EUR")
	if errs := checkRow(pass, "EUR", fixedNow, nil); containsCode(errs, CodeDateImplausible) {
		t.Errorf("clean row failed date check: %v", errs)
	}

	tooOld := baseRow(t, "EUR")
	tooOld.BookedOn = "01.01.2000"
	errs := checkRow(tooOld, "EUR", fixedNow, nil)
	if !containsCode(errs, CodeDateImplausible) {
		t.Errorf("a date 26 years old should fail: %v", errs)
	}
	if line := lineFor(errs, CodeDateImplausible); line != tooOld.LineNo {
		t.Errorf("error line = %d, want %d", line, tooOld.LineNo)
	}

	unparseable := baseRow(t, "EUR")
	unparseable.BookedOn = "not a date"
	if errs := checkRow(unparseable, "EUR", fixedNow, nil); !containsCode(errs, CodeDateImplausible) {
		t.Error("an unparseable date should fail as implausible")
	}
}

func TestAmountUnparseable(t *testing.T) {
	pass := baseRow(t, "EUR")
	if errs := checkRow(pass, "EUR", fixedNow, nil); containsCode(errs, CodeAmountUnparseable) {
		t.Errorf("clean row failed amount check: %v", errs)
	}

	dirty := baseRow(t, "EUR")
	dirty.DebitRaw = "not a number"
	errs := checkRow(dirty, "EUR", fixedNow, nil)
	if !containsCode(errs, CodeAmountUnparseable) {
		t.Error("an unparseable debit should fail")
	}
	if field := fieldFor(errs, CodeAmountUnparseable); field != "debit" {
		t.Errorf("field = %q, want debit", field)
	}
}

func TestCurrencyUnknown(t *testing.T) {
	if e := checkCurrency("EUR"); e != nil {
		t.Errorf("a real ISO-4217 code failed: %v", e)
	}
	e := checkCurrency("XYZ")
	if e == nil || e.Code != CodeCurrencyUnknown {
		t.Errorf("an unknown code should fail as %s, got %v", CodeCurrencyUnknown, e)
	}
}

func TestDebitAndCreditBothSet(t *testing.T) {
	pass := baseRow(t, "EUR") // debit 0, credit 5000 -- ordinary.
	if errs := checkRow(pass, "EUR", fixedNow, nil); containsCode(errs, CodeDebitAndCreditBothSet) {
		t.Errorf("an ordinary row (one zero side) failed: %v", errs)
	}

	dirty := baseRow(t, "EUR")
	dirty.Debit = mustMoney(t, "EUR", 100)
	dirty.Credit = mustMoney(t, "EUR", 200)
	if errs := checkRow(dirty, "EUR", fixedNow, nil); !containsCode(errs, CodeDebitAndCreditBothSet) {
		t.Error("both sides non-zero should fail")
	}
}

func TestReplacementCharacter(t *testing.T) {
	pass := baseRow(t, "EUR")
	if errs := checkRow(pass, "EUR", fixedNow, nil); containsCode(errs, CodeReplacementCharacter) {
		t.Errorf("clean row failed: %v", errs)
	}

	dirty := baseRow(t, "EUR")
	dirty.Description = "corrupted � text"
	if errs := checkRow(dirty, "EUR", fixedNow, nil); !containsCode(errs, CodeReplacementCharacter) {
		t.Error("U+FFFD in description should fail")
	}
}

func TestDescriptionMissing(t *testing.T) {
	pass := baseRow(t, "EUR")
	if errs := checkRow(pass, "EUR", fixedNow, nil); containsCode(errs, CodeDescriptionMissing) {
		t.Errorf("clean row failed: %v", errs)
	}

	dirty := baseRow(t, "EUR")
	dirty.Description = "   "
	if errs := checkRow(dirty, "EUR", fixedNow, nil); !containsCode(errs, CodeDescriptionMissing) {
		t.Error("a blank description should fail")
	}
}

func TestBalanceMismatchCheck(t *testing.T) {
	clean := &Statement{
		Currency: "EUR", HasDeclaredBalance: true,
		Opening: mustMoney(t, "EUR", 10000), Closing: mustMoney(t, "EUR", 15000),
		Rows: []Row{baseRow(t, "EUR")}, // Debit 0, Credit 5000.
	}
	result := ValidateStatement(clean, fixedNow)
	if result.BalanceCheckPassed == nil || !*result.BalanceCheckPassed {
		t.Errorf("a reconciling statement should pass; got %+v", result)
	}
	if containsWarningCode(result.Warnings, CodeBalanceMismatch) {
		t.Error("a reconciling statement should carry no balance_mismatch warning")
	}
	if result.Outcome != OutcomeValid {
		t.Errorf("outcome = %s, want valid", result.Outcome)
	}

	// Task 6.5: one minor unit off. BalanceCheck's own sign convention is
	// expected-minus-closing (core/internal/ingest/priorbank.go), so a
	// closing balance one unit *larger* than what the rows account for is
	// -1, not +1 -- this is 2.2's existing arithmetic, not a choice made
	// here.
	dirty := &Statement{
		Currency: "EUR", HasDeclaredBalance: true,
		Opening: mustMoney(t, "EUR", 10000), Closing: mustMoney(t, "EUR", 15001),
		Rows: []Row{baseRow(t, "EUR")},
	}
	result = ValidateStatement(dirty, fixedNow)
	if result.BalanceCheckPassed == nil || *result.BalanceCheckPassed {
		t.Errorf("a statement off by one minor unit should not pass; got %+v", result)
	}
	w := warningFor(result.Warnings, CodeBalanceMismatch)
	if w == nil || w.Balance == nil {
		t.Fatalf("want a balance_mismatch warning with detail, got %+v", result.Warnings)
	}
	if w.Balance.Difference != -1 {
		t.Errorf("difference = %d, want -1", w.Balance.Difference)
	}
	if result.Outcome != OutcomeValidWithWarnings {
		t.Errorf("outcome = %s, want valid_with_warnings", result.Outcome)
	}
}

// Task 6.6: no balances declared.
func TestNoBalancesDeclaredIsNullNotFailed(t *testing.T) {
	st := &Statement{
		Currency: "EUR", HasDeclaredBalance: false,
		Rows: []Row{{LineNo: 1, BookedOn: "01.01.2026", Description: "x",
			Debit: mustMoney(t, "EUR", 0), Credit: mustMoney(t, "EUR", 100),
			DebitRaw: "0,00", CreditRaw: "1,00"}},
	}
	result := ValidateStatement(st, fixedNow)
	if result.BalanceCheckPassed != nil {
		t.Errorf("BalanceCheckPassed = %v, want nil (no balances declared)", *result.BalanceCheckPassed)
	}
	if result.Outcome == OutcomeRejected {
		t.Error("no declared balance must not itself reject the batch")
	}
}

// Task 6.12: non-base currency reconciles in its own currency.
func TestNonBaseCurrencyReconcilesInItsOwnCurrency(t *testing.T) {
	row := baseRow(t, "PLN")
	row.Credit, row.CreditRaw = mustMoney(t, "PLN", 50000), "500,00"
	st := &Statement{
		Currency: "PLN", HasDeclaredBalance: true,
		Opening: mustMoney(t, "PLN", 100000), Closing: mustMoney(t, "PLN", 150000),
		Rows: []Row{row},
	}
	result := ValidateStatement(st, fixedNow)
	if result.BalanceCheckPassed == nil || !*result.BalanceCheckPassed {
		t.Errorf("a PLN statement reconciling in PLN should pass regardless of the org's base currency; got %+v", result)
	}
	w := warningFor(result.Warnings, CodeBalanceMismatch)
	if w != nil && w.Balance != nil && w.Balance.Currency != "PLN" {
		t.Errorf("balance detail currency = %q, want PLN -- converting before this check would be a wrong answer that looks right", w.Balance.Currency)
	}
}

func TestPeriodGap(t *testing.T) {
	inBounds := &Statement{
		Currency: "EUR", PeriodFrom: "01.06.2026", PeriodTo: "30.06.2026",
		Rows: []Row{baseRow(t, "EUR")}, // BookedOn 15.06.2026
	}
	if result := ValidateStatement(inBounds, fixedNow); containsWarningCode(result.Warnings, CodePeriodGap) {
		t.Errorf("a row inside the declared period should not warn: %+v", result.Warnings)
	}

	row := baseRow(t, "EUR")
	row.BookedOn = "05.07.2026" // after the declared period ends
	outOfBounds := &Statement{
		Currency: "EUR", PeriodFrom: "01.06.2026", PeriodTo: "30.06.2026",
		Rows: []Row{row},
	}
	if result := ValidateStatement(outOfBounds, fixedNow); !containsWarningCode(result.Warnings, CodePeriodGap) {
		t.Error("a row outside the declared period should warn")
	}

	noPeriodDeclared := &Statement{Currency: "EUR", Rows: []Row{baseRow(t, "EUR")}}
	if result := ValidateStatement(noPeriodDeclared, fixedNow); containsWarningCode(result.Warnings, CodePeriodGap) {
		t.Error("no declared period means nothing to check")
	}
}

func TestDuplicateBankReference(t *testing.T) {
	unique := &Statement{Currency: "EUR", Rows: []Row{
		func() Row { r := baseRow(t, "EUR"); r.DocumentNo = "1"; return r }(),
		func() Row { r := baseRow(t, "EUR"); r.DocumentNo = "2"; return r }(),
	}}
	if result := ValidateStatement(unique, fixedNow); containsWarningCode(result.Warnings, CodeDuplicateBankRef) {
		t.Error("distinct document numbers should not warn")
	}

	duplicated := &Statement{Currency: "EUR", Rows: []Row{
		func() Row { r := baseRow(t, "EUR"); r.DocumentNo = "1"; return r }(),
		func() Row { r := baseRow(t, "EUR"); r.DocumentNo = "1"; return r }(),
	}}
	if result := ValidateStatement(duplicated, fixedNow); !containsWarningCode(result.Warnings, CodeDuplicateBankRef) {
		t.Error("a repeated document number should warn")
	}
}

// A real redacted Priorbank export reuses "N док." across genuinely
// distinct transactions on different dates -- discovered running the real
// fixtures through this check for the first time, where it produced a false
// positive on data that reconciles exactly. DocumentNo alone is not the key;
// TestDuplicateBankReference's "duplicated" case above still exercises the
// real duplicate this check exists to catch.
func TestSameDocumentNoOnDifferentDatesIsNotADuplicate(t *testing.T) {
	first := baseRow(t, "EUR")
	first.DocumentNo, first.BookedOn = "40", "04.01.2026"
	second := baseRow(t, "EUR")
	second.DocumentNo, second.BookedOn = "40", "05.01.2026"

	st := &Statement{Currency: "EUR", Rows: []Row{first, second}}
	if result := ValidateStatement(st, fixedNow); containsWarningCode(result.Warnings, CodeDuplicateBankRef) {
		t.Error("the same document number on two different dates is not a duplicate")
	}
}

func TestRowCountMismatch(t *testing.T) {
	two := 2
	matches := &Statement{Currency: "EUR", DeclaredRowCount: &two, Rows: []Row{baseRow(t, "EUR"), baseRow(t, "EUR")}}
	if result := ValidateStatement(matches, fixedNow); containsWarningCode(result.Warnings, CodeRowCountMismatch) {
		t.Error("a matching declared count should not warn")
	}

	three := 3
	mismatches := &Statement{Currency: "EUR", DeclaredRowCount: &three, Rows: []Row{baseRow(t, "EUR"), baseRow(t, "EUR")}}
	if result := ValidateStatement(mismatches, fixedNow); !containsWarningCode(result.Warnings, CodeRowCountMismatch) {
		t.Error("a mismatched declared count should warn")
	}

	noneDeclared := &Statement{Currency: "EUR", Rows: []Row{baseRow(t, "EUR")}}
	if result := ValidateStatement(noneDeclared, fixedNow); containsWarningCode(result.Warnings, CodeRowCountMismatch) {
		t.Error("no declared count means nothing to check")
	}
}

// Task 6.3: line numbers survive into the report.
func TestLineNumbersSurviveIntoTheReport(t *testing.T) {
	st := &Statement{
		Currency: "EUR",
		Rows: []Row{
			baseRow(t, "EUR"),
			func() Row { r := baseRow(t, "EUR"); r.LineNo = 2847; r.BookedOn = "bad"; return r }(),
		},
	}
	result := ValidateStatement(st, fixedNow)
	if line := lineFor(result.Errors, CodeDateImplausible); line != 2847 {
		t.Errorf("error line = %d, want 2847", line)
	}
}

// Task 6.9: no sentences in report_jsonb.
var codeShape = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func TestReportJSONCarriesNoSentences(t *testing.T) {
	st := &Statement{
		Currency: "EUR", HasDeclaredBalance: true,
		Opening: mustMoney(t, "EUR", 0), Closing: mustMoney(t, "EUR", 1),
		Rows: []Row{func() Row { r := baseRow(t, "EUR"); r.Description = ""; return r }()},
	}
	result := ValidateStatement(st, fixedNow)
	raw, err := BuildReportJSON(result)
	if err != nil {
		t.Fatalf("BuildReportJSON: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("report_jsonb did not parse as JSON: %v", err)
	}

	for _, e := range doc["errors"].([]any) {
		code := e.(map[string]any)["code"].(string)
		if !codeShape.MatchString(code) {
			t.Errorf("error code %q is not a bare code -- looks like a sentence", code)
		}
	}
	for _, w := range doc["warnings"].([]any) {
		code := w.(map[string]any)["code"].(string)
		if !codeShape.MatchString(code) {
			t.Errorf("warning code %q is not a bare code -- looks like a sentence", code)
		}
	}
}

// --- helpers --------------------------------------------------------------

func containsCode(errs []ValidationError, code string) bool {
	for _, e := range errs {
		if e.Code == code {
			return true
		}
	}
	return false
}

func lineFor(errs []ValidationError, code string) int {
	for _, e := range errs {
		if e.Code == code {
			return e.Line
		}
	}
	return -1
}

func fieldFor(errs []ValidationError, code string) string {
	for _, e := range errs {
		if e.Code == code {
			return e.Field
		}
	}
	return ""
}

func containsWarningCode(warns []ValidationWarning, code string) bool {
	return warningFor(warns, code) != nil
}

func warningFor(warns []ValidationWarning, code string) *ValidationWarning {
	for i, w := range warns {
		if w.Code == code {
			return &warns[i]
		}
	}
	return nil
}
