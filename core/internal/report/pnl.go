// Package report computes the management P&L.
//
// The calculation takes rows and returns a report. It opens no database, reads
// no clock and holds no state, for the same reason the classification engine
// does not: a report that is a pure function of its inputs can be reproduced
// from stored versions six months later, and an accountant will ask. It also
// means the whole of the arithmetic is tested without Postgres, and the query
// is tested separately for the things queries get wrong.
package report

import (
	"fmt"
	"sort"

	"github.com/MyauDev/vekst/core/internal/money"
)

// The sections a transaction can land on, and the five lines computed from
// them. Codes rather than names: `categories.formula` holds prose over names --
// 'NET SALES - CS' -- which nothing resolves and nothing validates, so a rename
// would produce a wrong gross margin rather than an error. The prose stays as
// the label a screen prints; this is what the computer follows.
const (
	NetSales = "01"
	CS       = "02"
	OCS      = "03"
	OPEX     = "04"
	OIE      = "05"
	FR       = "06"
	CIT      = "07"
	CAPEX    = "08"
	OutOfPNL = "09"

	GM  = "91"
	NM  = "92"
	CM  = "93"
	IBT = "94"
	NI  = "95"
)

// computedLine is one step of the chain, in the signs the ledger stores.
//
// Read as accounting, `GM = NET SALES - CS`. Read as arithmetic over signed
// amounts it is an addition, because a cost is already negative. That inversion
// is the kind of thing that belongs in one place with a name on it: `minus`
// says "this operand is a cost", and the code below adds it.
type computedLine struct {
	Code    string
	Label   string
	Formula string // what the taxonomy stores, for a reader
	Plus    []string
	Minus   []string
}

// Chain is the report, in order. Each line may name an earlier computed line,
// which is why the order is fixed and not a set.
var Chain = []computedLine{
	{GM, "GM", "NET SALES - CS", []string{NetSales}, []string{CS}},
	{NM, "NM", "GM - OCS", []string{GM}, []string{OCS}},
	{CM, "CM", "NM - OPEX - OIE", []string{NM}, []string{OPEX, OIE}},
	{IBT, "IBT", "CM - FR", []string{CM}, []string{FR}},
	{NI, "NI", "IBT - CIT", []string{IBT}, []string{CIT}},
}

// Sections are the lines that receive transactions, in report order.
var Sections = []string{NetSales, CS, OCS, OPEX, OIE, FR, CIT}

// Basis is which half of the business a report is computed from. A line is
// computed from one of these and never from both: mixing them counts an invoice
// and its payment twice.
type Basis string

const (
	BasisLedger Basis = "ledger"
	BasisBank   Basis = "bank"
)

// Bucket names what a report could not include. Every transaction in the period
// lands on exactly one line or in exactly one bucket, which `Compute` asserts.
type Bucket string

const (
	// BucketUnclassified is the one that matters. A P&L summing only what was
	// classified describes a smaller business than the one that exists, and
	// looks finished while doing it.
	BucketUnclassified Bucket = "unclassified"

	// BucketNonPNL is CAPEX and OUT OF P&L: classified, and deliberately not
	// on any line.
	BucketNonPNL Bucket = "non_pnl"

	// BucketUnallocated is known to be payroll and not known to be any
	// department's. Attributing it would be a guess printed as a figure.
	BucketUnallocated Bucket = "unallocated"

	// BucketOtherBasis is the reconciliation evidence: rows of the source kind
	// this report is not computed from, counted rather than dropped.
	BucketOtherBasis Bucket = "other_basis"
)

// Row is what one transaction contributes. No identifiers, no pointers,
// nothing to read further: what is not here cannot influence the answer, which
// is what makes a report reproducible.
type Row struct {
	// Period is the column this row falls in, formatted by the caller --
	// "2026-03" for a month. The calculation does no date arithmetic, because
	// a booking date has no time zone and inventing one here would be the
	// wrong place to get it wrong.
	Period string

	// CategoryCode is empty when no live classification exists. That is not a
	// missing value: it is the fact that puts the row in the unclassified
	// bucket.
	CategoryCode string

	// Section is the category's level-1 ancestor, read from `pnl_section`.
	Section string

	// Amount is signed, in the organisation's base currency: money in
	// positive, money out negative (ARCHITECTURE.md §5.0).
	Amount money.Money

	IsPNL              bool
	RequiresAllocation bool

	// SourceKind is the row's own. A row whose kind is not the report's basis
	// is counted in BucketOtherBasis and reaches no line.
	SourceKind Basis
}

// Spec is what the caller asked for.
type Spec struct {
	Basis Basis

	// Periods in report order, including any with no rows: a month missing
	// from a report is indistinguishable from a month with no trade, and only
	// one of those is a business fact.
	Periods []string

	// BaseCurrency labels every figure. A total with no code beside it is not
	// money.
	BaseCurrency string

	TaxonomyVersion  string
	RulesetVersion   string
	EngineVersion    string
	NormalizeVersion string
}

// Figure is one cell: an amount, and its share of revenue where that means
// anything.
type Figure struct {
	Amount money.Money

	// PercentOfRevenue is a ratio and not money, which is why a float is
	// correct here and nowhere else in this package. Absent when the period
	// has no revenue -- neither zero nor infinity, because both would be read
	// as a fact.
	PercentOfRevenue    float64
	HasPercentOfRevenue bool
}

// Line is one row of the table.
type Line struct {
	Code  string
	Label string

	// Formula is what the taxonomy stores, for a reader. Empty on a section.
	Formula string

	// Computed distinguishes a line summed from transactions from one derived
	// by arithmetic. A drill-down opens the two differently: a section has
	// transactions, a computed line has operands.
	Computed bool

	// ByPeriod is indexed by Spec.Periods.
	ByPeriod []Figure
	Total    Figure
}

// Report is the table plus everything it could not include.
type Report struct {
	Spec    Spec
	Lines   []Line
	Buckets map[Bucket][]money.Money // indexed by Spec.Periods
	Totals  map[Bucket]money.Money
}

// Compute turns rows into the report.
//
// Costs come out positive. A cost section sums to a negative number in the
// store, and an owner reading "OPEX -412,000" beside "NET SALES 1,200,000" is
// reading a spreadsheet rather than a report. The sign is carried by the line's
// role, and this is the one place the inversion happens.
func Compute(rows []Row, spec Spec) (Report, error) {
	if spec.BaseCurrency == "" {
		return Report{}, fmt.Errorf("report: no base currency: a figure with no code beside it is not money")
	}
	if len(spec.Periods) == 0 {
		return Report{}, fmt.Errorf("report: no periods requested")
	}

	index := make(map[string]int, len(spec.Periods))
	for i, p := range spec.Periods {
		index[p] = i
	}

	// Section totals in stored signs, and the buckets beside them.
	sums := map[string][]int64{}
	for _, s := range Sections {
		sums[s] = make([]int64, len(spec.Periods))
	}
	buckets := map[Bucket][]int64{
		BucketUnclassified: make([]int64, len(spec.Periods)),
		BucketNonPNL:       make([]int64, len(spec.Periods)),
		BucketUnallocated:  make([]int64, len(spec.Periods)),
		BucketOtherBasis:   make([]int64, len(spec.Periods)),
	}

	for _, r := range rows {
		i, ok := index[r.Period]
		if !ok {
			// A row outside the requested range would otherwise vanish into a
			// total that does not include its column, which is the same
			// failure as dropping it.
			return Report{}, fmt.Errorf("report: row in period %q, which was not requested", r.Period)
		}
		if r.Amount.CurrencyCode != spec.BaseCurrency {
			return Report{}, fmt.Errorf(
				"report: row in %s, but the report is in %s -- amounts are summed in one currency or not at all",
				r.Amount.CurrencyCode, spec.BaseCurrency)
		}

		switch {
		case r.SourceKind != spec.Basis:
			buckets[BucketOtherBasis][i] += r.Amount.MinorUnits
		case r.CategoryCode == "":
			buckets[BucketUnclassified][i] += r.Amount.MinorUnits
		case !r.IsPNL:
			buckets[BucketNonPNL][i] += r.Amount.MinorUnits
		case r.RequiresAllocation:
			buckets[BucketUnallocated][i] += r.Amount.MinorUnits
		default:
			s, ok := sums[r.Section]
			if !ok {
				return Report{}, fmt.Errorf(
					"report: category %s is in section %q, which is not a P&L section",
					r.CategoryCode, r.Section)
			}
			s[i] += r.Amount.MinorUnits
		}
	}

	// The chain, in stored signs. A cost operand is negative, so subtracting it
	// as accounting means adding it as arithmetic.
	computed := map[string][]int64{}
	value := func(code string) []int64 {
		if s, ok := sums[code]; ok {
			return s
		}
		return computed[code]
	}
	for _, line := range Chain {
		out := make([]int64, len(spec.Periods))
		for _, code := range line.Plus {
			addInto(out, value(code))
		}
		for _, code := range line.Minus {
			addInto(out, value(code))
		}
		computed[line.Code] = out
	}

	revenue := sums[NetSales]
	report := Report{
		Spec:    spec,
		Buckets: map[Bucket][]money.Money{},
		Totals:  map[Bucket]money.Money{},
	}

	for _, code := range Sections {
		report.Lines = append(report.Lines, buildLine(
			code, sectionLabel(code), "", false, present(code, sums[code]), revenue, spec))
	}
	for _, line := range Chain {
		report.Lines = append(report.Lines, buildLine(
			line.Code, line.Label, line.Formula, true, computed[line.Code], revenue, spec))
	}

	for name, series := range buckets {
		report.Buckets[name] = toMoney(series, spec.BaseCurrency)
		report.Totals[name] = money.Money{CurrencyCode: spec.BaseCurrency, MinorUnits: sum(series)}
	}
	return report, nil
}

// present flips a cost section so the page reads the way an owner reads it.
// NET SALES is money in and already positive; every other section is a cost.
func present(code string, series []int64) []int64 {
	if code == NetSales {
		return series
	}
	out := make([]int64, len(series))
	for i, v := range series {
		out[i] = -v
	}
	return out
}

func buildLine(code, label, formula string, computed bool, series, revenue []int64, spec Spec) Line {
	line := Line{Code: code, Label: label, Formula: formula, Computed: computed}
	for i, v := range series {
		line.ByPeriod = append(line.ByPeriod, figure(v, revenue[i], spec.BaseCurrency))
	}
	line.Total = figure(sum(series), sum(revenue), spec.BaseCurrency)
	return line
}

func figure(amount, revenue int64, currency string) Figure {
	f := Figure{Amount: money.Money{CurrencyCode: currency, MinorUnits: amount}}
	if revenue != 0 {
		f.PercentOfRevenue = float64(amount) / float64(revenue) * 100
		f.HasPercentOfRevenue = true
	}
	return f
}

func addInto(dst, src []int64) {
	for i := range dst {
		dst[i] += src[i]
	}
}

func sum(series []int64) int64 {
	var total int64
	for _, v := range series {
		total += v
	}
	return total
}

func toMoney(series []int64, currency string) []money.Money {
	out := make([]money.Money, len(series))
	for i, v := range series {
		out[i] = money.Money{CurrencyCode: currency, MinorUnits: v}
	}
	return out
}

var sectionLabels = map[string]string{
	NetSales: "NET SALES", CS: "CS", OCS: "OCS", OPEX: "OPEX",
	OIE: "OIE", FR: "FR", CIT: "CIT", CAPEX: "CAPEX", OutOfPNL: "OUT OF P&L",
}

func sectionLabel(code string) string { return sectionLabels[code] }

// PeriodsBetween produces the month columns of a closed range, including the
// months with no rows.
func PeriodsBetween(from, to string) ([]string, error) {
	if from > to {
		return nil, fmt.Errorf("report: period %s is after %s", from, to)
	}
	var out []string
	var y, m int
	if _, err := fmt.Sscanf(from, "%4d-%2d", &y, &m); err != nil {
		return nil, fmt.Errorf("report: %q is not a month: %w", from, err)
	}
	for {
		p := fmt.Sprintf("%04d-%02d", y, m)
		if p > to {
			break
		}
		out = append(out, p)
		if m++; m > 12 {
			y, m = y+1, 1
		}
	}
	sort.Strings(out)
	return out, nil
}
