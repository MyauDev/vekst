package report_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/report"
)

const ccy = "NOK"

func spec(periods ...string) report.Spec {
	return report.Spec{
		Basis:            report.BasisBank,
		Periods:          periods,
		BaseCurrency:     ccy,
		TaxonomyVersion:  "v1",
		RulesetVersion:   "v1",
		EngineVersion:    "human",
		NormalizeVersion: "v1",
	}
}

// row writes an amount in the signs the ledger stores: money in positive,
// money out negative.
func row(period, code, section string, minor int64) report.Row {
	return report.Row{
		Period:       period,
		CategoryCode: code,
		Section:      section,
		Amount:       money.Money{CurrencyCode: ccy, MinorUnits: minor},
		IsPNL:        true,
		SourceKind:   report.BasisBank,
	}
}

func lineOf(t *testing.T, r report.Report, code string) report.Line {
	t.Helper()
	for _, l := range r.Lines {
		if l.Code == code {
			return l
		}
	}
	t.Fatalf("no line %q in the report", code)
	return report.Line{}
}

func compute(t *testing.T, rows []report.Row, s report.Spec) report.Report {
	t.Helper()
	r, err := report.Compute(rows, s)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	return r
}

// ---------------------------------------------------------------------------
// Task 3.1 -- the chain
// ---------------------------------------------------------------------------

func TestTheChainProducesTheExpectedFigures(t *testing.T) {
	// One transaction per section. Revenue in, everything else out.
	rows := []report.Row{
		row("2026-03", "0101", report.NetSales, 1_000_000),
		row("2026-03", "0201", report.CS, -300_000),
		row("2026-03", "0301", report.OCS, -50_000),
		row("2026-03", "0404", report.OPEX, -200_000),
		row("2026-03", "0501", report.OIE, -20_000),
		row("2026-03", "0601", report.FR, -10_000),
		row("2026-03", "0701", report.CIT, -84_000),
	}
	r := compute(t, rows, spec("2026-03"))

	// GM = 1,000,000 - 300,000. NM = GM - 50,000. CM = NM - 200,000 - 20,000.
	// IBT = CM - 10,000. NI = IBT - 84,000.
	for _, tc := range []struct {
		code string
		want int64
	}{
		{report.GM, 700_000},
		{report.NM, 650_000},
		{report.CM, 430_000},
		{report.IBT, 420_000},
		{report.NI, 336_000},
	} {
		if got := lineOf(t, r, tc.code).Total.Amount.MinorUnits; got != tc.want {
			t.Errorf("%s = %d, want %d", tc.code, got, tc.want)
		}
	}
}

// Task 3.3. A cost is negative in the store and positive on the page: an owner
// reading "OPEX -412,000" beside "NET SALES 1,200,000" is reading a
// spreadsheet, not a report.
func TestCostsAreNegativeInTheStoreAndPositiveOnThePage(t *testing.T) {
	r := compute(t, []report.Row{
		row("2026-03", "0101", report.NetSales, 1_000_000),
		row("2026-03", "0404", report.OPEX, -412_000),
	}, spec("2026-03"))

	if got := lineOf(t, r, report.OPEX).Total.Amount.MinorUnits; got != 412_000 {
		t.Errorf("OPEX prints %d, want a positive 412000", got)
	}
	if got := lineOf(t, r, report.NetSales).Total.Amount.MinorUnits; got != 1_000_000 {
		t.Errorf("NET SALES prints %d, want 1000000", got)
	}
	// And the arithmetic still used the stored sign: CM is revenue less cost.
	if got := lineOf(t, r, report.CM).Total.Amount.MinorUnits; got != 588_000 {
		t.Errorf("CM = %d, want 588000 -- the inversion leaked into the chain", got)
	}
}

// Task 3.2. The taxonomy stores the formula as prose over names. It is a label,
// and this asserts the label still describes what the code does.
func TestTheChainMatchesTheStoredFormulas(t *testing.T) {
	want := map[string]string{
		report.GM:  "NET SALES - CS",
		report.NM:  "GM - OCS",
		report.CM:  "NM - OPEX - OIE",
		report.IBT: "CM - FR",
		report.NI:  "IBT - CIT",
	}
	if len(report.Chain) != len(want) {
		t.Fatalf("the chain has %d lines, the taxonomy seeds %d", len(report.Chain), len(want))
	}
	for _, line := range report.Chain {
		if line.Formula != want[line.Code] {
			t.Errorf("%s: formula %q, taxonomy says %q", line.Code, line.Formula, want[line.Code])
		}
	}

	// Every operand is a section or a line computed earlier: a chain that
	// referred forwards would read an empty series and produce a silent zero.
	seen := map[string]bool{}
	for _, s := range report.Sections {
		seen[s] = true
	}
	for _, line := range report.Chain {
		for _, code := range append(append([]string{}, line.Plus...), line.Minus...) {
			if !seen[code] {
				t.Errorf("%s names %s, which is neither a section nor computed before it",
					line.Code, code)
			}
		}
		seen[line.Code] = true
	}
}

// ---------------------------------------------------------------------------
// Task 3.8 -- nothing is lost
// ---------------------------------------------------------------------------

func TestEveryRowLandsOnALineOrInABucket(t *testing.T) {
	rows := []report.Row{
		row("2026-03", "0101", report.NetSales, 900_000),
		row("2026-03", "0404", report.OPEX, -100_000),
		{Period: "2026-03", Amount: money.Money{CurrencyCode: ccy, MinorUnits: -70_000},
			SourceKind: report.BasisBank}, // unclassified
		{Period: "2026-03", CategoryCode: "08", Section: report.CAPEX,
			Amount: money.Money{CurrencyCode: ccy, MinorUnits: -50_000}, SourceKind: report.BasisBank},
		{Period: "2026-03", CategoryCode: "0403", Section: report.OPEX, IsPNL: true,
			RequiresAllocation: true,
			Amount:             money.Money{CurrencyCode: ccy, MinorUnits: -30_000}, SourceKind: report.BasisBank},
		{Period: "2026-03", CategoryCode: "0101", Section: report.NetSales, IsPNL: true,
			Amount: money.Money{CurrencyCode: ccy, MinorUnits: 11_000}, SourceKind: report.BasisLedger},
	}
	r := compute(t, rows, spec("2026-03"))

	var everything int64
	for _, row := range rows {
		everything += row.Amount.MinorUnits
	}

	var sections int64
	for _, code := range report.Sections {
		// Sections print inverted, so read them back in stored signs.
		v := lineOf(t, r, code).Total.Amount.MinorUnits
		if code != report.NetSales {
			v = -v
		}
		sections += v
	}
	for _, b := range r.Totals {
		sections += b.MinorUnits
	}

	if sections != everything {
		t.Errorf("lines plus buckets = %d, the rows sum to %d -- %d went missing",
			sections, everything, everything-sections)
	}
	for name, want := range map[report.Bucket]int64{
		report.BucketUnclassified: -70_000,
		report.BucketNonPNL:       -50_000,
		report.BucketUnallocated:  -30_000,
		report.BucketOtherBasis:   11_000,
	} {
		if got := r.Totals[name].MinorUnits; got != want {
			t.Errorf("bucket %s = %d, want %d", name, got, want)
		}
	}
}

// A ledger row in a bank report reaches no line. Mixing them counts an invoice
// and its payment twice, which is the most likely way this product prints a
// wrong number.
func TestTheOtherBasisReachesNoLine(t *testing.T) {
	r := compute(t, []report.Row{
		{Period: "2026-03", CategoryCode: "0101", Section: report.NetSales, IsPNL: true,
			Amount: money.Money{CurrencyCode: ccy, MinorUnits: 500_000}, SourceKind: report.BasisLedger},
	}, spec("2026-03"))

	if got := lineOf(t, r, report.NetSales).Total.Amount.MinorUnits; got != 0 {
		t.Errorf("NET SALES = %d from a ledger row in a bank report", got)
	}
	if got := r.Totals[report.BucketOtherBasis].MinorUnits; got != 500_000 {
		t.Errorf("other basis = %d, want it counted rather than dropped", got)
	}
}

// ---------------------------------------------------------------------------
// Tasks 3.4 to 3.7 -- money, percentages and periods
// ---------------------------------------------------------------------------

// Exponents never appear in this package: every amount is already in the
// organisation's base currency and is an integer count of its minor units. That
// is the property being asserted -- a value beyond float64's exact range comes
// through unchanged.
func TestAmountsBeyondFloatPrecisionSurvive(t *testing.T) {
	const big = 9_007_199_254_740_993 // 2^53 + 1
	r := compute(t, []report.Row{
		row("2026-03", "0101", report.NetSales, big),
	}, spec("2026-03"))

	if got := lineOf(t, r, report.NetSales).Total.Amount.MinorUnits; got != big {
		t.Errorf("NET SALES = %d, want %d -- a float64 cannot hold this exactly", got, big)
	}
}

func TestMoneyIsNeverAFloat(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(report.Figure{}),
		reflect.TypeOf(report.Row{}),
		reflect.TypeOf(report.Line{}),
	} {
		for i := range typ.NumField() {
			f := typ.Field(i)
			if f.Type.Kind() != reflect.Float64 && f.Type.Kind() != reflect.Float32 {
				continue
			}
			// One exception, and it is a ratio rather than money.
			if typ == reflect.TypeOf(report.Figure{}) && f.Name == "PercentOfRevenue" {
				continue
			}
			t.Errorf("%s.%s is a float; money is int64 minor units plus a code",
				typ.Name(), f.Name)
		}
	}
}

func TestAPercentageOfNoRevenueIsAbsent(t *testing.T) {
	r := compute(t, []report.Row{
		row("2026-03", "0404", report.OPEX, -100_000),
	}, spec("2026-03"))

	opex := lineOf(t, r, report.OPEX).Total
	if opex.HasPercentOfRevenue {
		t.Errorf("percent of revenue = %v with no revenue; it must be absent, not zero or infinite",
			opex.PercentOfRevenue)
	}
	if math.IsInf(opex.PercentOfRevenue, 0) || math.IsNaN(opex.PercentOfRevenue) {
		t.Errorf("percent of revenue = %v", opex.PercentOfRevenue)
	}
}

func TestPercentOfRevenueIsComputedAgainstRevenue(t *testing.T) {
	r := compute(t, []report.Row{
		row("2026-03", "0101", report.NetSales, 1_000_000),
		row("2026-03", "0404", report.OPEX, -250_000),
	}, spec("2026-03"))

	opex := lineOf(t, r, report.OPEX).Total
	if !opex.HasPercentOfRevenue || math.Abs(opex.PercentOfRevenue-25) > 1e-9 {
		t.Errorf("OPEX %% of revenue = %v, want 25", opex.PercentOfRevenue)
	}
}

func TestAnEmptyPeriodIsAZeroColumn(t *testing.T) {
	r := compute(t, []report.Row{
		row("2026-03", "0101", report.NetSales, 100),
	}, spec("2026-01", "2026-02", "2026-03"))

	line := lineOf(t, r, report.NetSales)
	if len(line.ByPeriod) != 3 {
		t.Fatalf("columns = %d, want 3 -- a month absent from a report reads as a month with no trade",
			len(line.ByPeriod))
	}
	for i, want := range []int64{0, 0, 100} {
		if got := line.ByPeriod[i].Amount.MinorUnits; got != want {
			t.Errorf("column %d = %d, want %d", i, got, want)
		}
	}
}

func TestARowOutsideTheRangeIsAnError(t *testing.T) {
	_, err := report.Compute([]report.Row{
		row("2026-04", "0101", report.NetSales, 100),
	}, spec("2026-03"))
	if err == nil {
		t.Fatal("a row outside the requested periods was accepted; it would vanish into a total")
	}
}

func TestASecondCurrencyIsAnError(t *testing.T) {
	_, err := report.Compute([]report.Row{
		{Period: "2026-03", CategoryCode: "0101", Section: report.NetSales, IsPNL: true,
			Amount: money.Money{CurrencyCode: "JPY", MinorUnits: 100}, SourceKind: report.BasisBank},
	}, spec("2026-03"))
	if err == nil {
		t.Fatal("a JPY row was summed into a NOK report")
	}
}

func TestPeriodsBetween(t *testing.T) {
	got, err := report.PeriodsBetween("2025-11", "2026-02")
	if err != nil {
		t.Fatalf("PeriodsBetween: %v", err)
	}
	want := []string{"2025-11", "2025-12", "2026-01", "2026-02"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if _, err := report.PeriodsBetween("2026-05", "2026-01"); err == nil {
		t.Error("a range running backwards was accepted")
	}
}
