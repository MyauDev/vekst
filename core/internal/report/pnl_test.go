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
		Basis:             report.BasisBank,
		Periods:           periods,
		BaseCurrency:      ccy,
		TaxonomyVersions:  []string{"v1"},
		RulesetVersions:   []string{"v1"},
		EngineVersions:    []string{"human"},
		NormalizeVersions: []string{"v1"},
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

func TestMonths(t *testing.T) {
	got, err := report.Months("2025-11", "2026-02")
	if err != nil {
		t.Fatalf("Months: %v", err)
	}
	want := []string{"2025-11", "2025-12", "2026-01", "2026-02"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if _, err := report.Months("2026-05", "2026-01"); err == nil {
		t.Error("a range running backwards was accepted")
	}
	for _, bad := range []string{"2026", "2026-13", "2026-00", "march", ""} {
		if _, err := report.Months(bad, "2026-12"); err == nil {
			t.Errorf("Months(%q) was accepted", bad)
		}
	}
}

// A quarter is three months folded, and the label says so. Columns stay in
// order and a quarter appears once however many of its months are in range --
// a half-quarter at the edge of a range is still that quarter's column.
func TestPeriodsFoldMonthsByGranularity(t *testing.T) {
	for _, c := range []struct {
		g        report.Granularity
		from, to string
		want     []string
	}{
		{report.Monthly, "2026-01", "2026-03", []string{"2026-01", "2026-02", "2026-03"}},
		{report.Quarterly, "2026-01", "2026-07", []string{"2026-Q1", "2026-Q2", "2026-Q3"}},
		{report.Quarterly, "2026-02", "2026-02", []string{"2026-Q1"}},
		{report.Yearly, "2025-11", "2026-02", []string{"2025", "2026"}},
	} {
		got, err := report.Periods(c.from, c.to, c.g)
		if err != nil {
			t.Fatalf("Periods(%s, %s, %s): %v", c.from, c.to, c.g, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Periods(%s, %s, %s) = %v, want %v", c.from, c.to, c.g, got, c.want)
		}
	}

	if _, err := report.Periods("2026-01", "2026-03", report.Granularity("fortnight")); err == nil {
		t.Error("an unknown granularity was accepted")
	}
}

// Every month of a range folds onto exactly one column of that range. A month
// that folded onto nothing would take its transactions out of the report
// without saying so, which is the one thing a period may never do.
func TestEveryMonthFoldsOntoAColumn(t *testing.T) {
	for _, g := range []report.Granularity{report.Monthly, report.Quarterly, report.Yearly} {
		months, err := report.Months("2024-01", "2026-12")
		if err != nil {
			t.Fatal(err)
		}
		periods, err := report.Periods("2024-01", "2026-12", g)
		if err != nil {
			t.Fatal(err)
		}
		known := map[string]bool{}
		for _, p := range periods {
			known[p] = true
		}
		for _, m := range months {
			label, err := g.Label(m)
			if err != nil {
				t.Fatalf("%s.Label(%s): %v", g, m, err)
			}
			if !known[label] {
				t.Errorf("%s: month %s folds onto %q, which is not a column", g, m, label)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Task 0.5 -- D-3, the output table structure
// ---------------------------------------------------------------------------

// The table is printed in one order and it is not "sections, then results".
// Each computed line follows the operands it consumes, so the reader never has
// to hold two figures in their head to see where a third came from.
func TestTheTableIsPrintedInReadingOrder(t *testing.T) {
	r := compute(t, nil, spec("2026-03"))

	want := []string{"01", "02", "91", "03", "92", "04", "05", "93", "06", "94", "07", "95"}
	got := make([]string, 0, len(r.Lines))
	for _, l := range r.Lines {
		got = append(got, l.Code)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("line order = %v, want %v", got, want)
	}

	// A computed line carries the prose a reader wants beside it; a section
	// has none, because there is no arithmetic to explain.
	for _, l := range r.Lines {
		if l.Computed != (l.Formula != "") {
			t.Errorf("line %s: computed=%v but formula=%q", l.Code, l.Computed, l.Formula)
		}
	}
}

// Every line in the printed order is one the report actually computes, and
// every line the report computes is printed. A section added to the taxonomy
// and forgotten here would otherwise vanish from the table while still being
// summed into the chain.
func TestTheOrderCoversEverySectionAndEveryComputedLine(t *testing.T) {
	inOrder := map[string]bool{}
	for _, c := range report.Order {
		if inOrder[c] {
			t.Errorf("code %s appears twice in Order", c)
		}
		inOrder[c] = true
	}

	for _, c := range report.Sections {
		if !inOrder[c] {
			t.Errorf("section %s is summed but never printed", c)
		}
		delete(inOrder, c)
	}
	for _, l := range report.Chain {
		if !inOrder[l.Code] {
			t.Errorf("computed line %s is never printed", l.Code)
		}
		delete(inOrder, l.Code)
	}
	for c := range inOrder {
		t.Errorf("Order prints %s, which is neither a section nor a computed line", c)
	}
}

// The buckets are printed too, and all four of them. One quietly dropped from
// BucketOrder would be computed, returned in the map, and never shown -- which
// is precisely the silent omission D6 exists to prevent.
func TestEveryBucketIsPrinted(t *testing.T) {
	r := compute(t, nil, spec("2026-03"))

	printed := map[report.Bucket]bool{}
	for _, b := range report.BucketOrder {
		if printed[b] {
			t.Errorf("bucket %s appears twice in BucketOrder", b)
		}
		printed[b] = true
	}
	for b := range r.Totals {
		if !printed[b] {
			t.Errorf("bucket %s is computed but never printed", b)
		}
	}
	for b := range printed {
		if _, ok := r.Totals[b]; !ok {
			t.Errorf("BucketOrder names %s, which the report does not compute", b)
		}
	}
}
