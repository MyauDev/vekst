package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// A cell of the report is addressed by what produced it, and by nothing else.
//
// The obvious design hands the client an opaque handle with each figure and
// takes it back to open the cell. It is tempting and it is wrong: a handle is
// state the server has to keep or sign, and a report whose drill-down needs
// server state is no longer reproducible from its inputs.
//
// So a cell is addressed by the four things that computed it -- entity, basis,
// period and line -- and this file turns those four into the selection both the
// figure and its drill-down read. One function used twice, rather than two
// WHERE clauses that agree today: a figure whose drill-down does not add up to
// it is the specific failure this change exists to make impossible, and two
// independently written predicates are how it happens.

// LineRef names the line of a cell: a taxonomy section, one of the five
// computed lines, or one of the four exclusion buckets. Exactly one of the two
// fields is set.
type LineRef struct {
	Code   string
	Bucket Bucket
}

// SectionLine addresses a line of the table.
func SectionLine(code string) LineRef { return LineRef{Code: code} }

// BucketLine addresses one of the four totals below it.
func BucketLine(b Bucket) LineRef { return LineRef{Bucket: b} }

// String is the wire form, and it is the code or the bucket name unchanged.
// Both vocabularies are closed and disjoint -- '01'..'09', '91'..'95' against
// 'unclassified', 'non_pnl', 'unallocated', 'other_basis' -- so no prefix is
// needed to tell them apart, and ParseLineRef checks membership rather than
// shape.
func (l LineRef) String() string {
	if l.Bucket != "" {
		return string(l.Bucket)
	}
	return l.Code
}

// IsBucket reports whether this addresses an exclusion total rather than a line
// of the table.
func (l LineRef) IsBucket() bool { return l.Bucket != "" }

// ParseLineRef reads the wire form, refusing anything that is not a line this
// report actually has. A cell that does not exist is a bad request, never an
// empty result: an empty drill-down of a figure of 412,000 reads as "these rows
// went missing".
func ParseLineRef(s string) (LineRef, error) {
	for _, b := range BucketOrder {
		if string(b) == s {
			return BucketLine(b), nil
		}
	}
	for _, c := range Order {
		if c == s {
			return SectionLine(c), nil
		}
	}
	for _, c := range NonPNLSections {
		if c == s {
			return SectionLine(c), nil
		}
	}
	return LineRef{}, fmt.Errorf("report: %q is not a line of this report", s)
}

// Operands are the lines a computed line is made of, in the order the formula
// names them, or nil for a line that receives transactions.
//
// GM has no transactions. It is NET SALES minus CS, and both of those have
// transactions. Opening GM therefore returns the two lines it is made of, each
// openable in turn -- one extra level of indirection, and the honest one. A
// drill-down that showed GM's "transactions" would have to invent a set, and
// the set it would invent is exactly what a reader would misread as a single
// category.
func Operands(code string) []string {
	for _, l := range Chain {
		if l.Code != code {
			continue
		}
		out := make([]string, 0, len(l.Plus)+len(l.Minus))
		out = append(out, l.Plus...)
		out = append(out, l.Minus...)
		return out
	}
	return nil
}

// Selection is the set of transactions one figure was summed from.
type Selection struct {
	EntityID uuid.UUID
	Basis    Basis

	// The cell's own dates, closed at both ends. Never the report's whole
	// range: a quarterly column of a report that starts in February covers
	// February and March, not January to March, and a drill-down reading the
	// unclipped quarter would return rows the figure never counted.
	From time.Time
	To   time.Time

	Line LineRef
}

// SelectionFor builds the selection for one cell of a report.
//
// It returns an error for a computed line: those have operands, not
// transactions, and a caller that has not checked Operands first is about to
// invent a set. Handing back an empty selection instead would make that
// invention silent.
func SelectionFor(req Request, period string, line LineRef) (Selection, error) {
	if !line.IsBucket() {
		if ops := Operands(line.Code); len(ops) > 0 {
			return Selection{}, fmt.Errorf(
				"report: %s is computed from %s and has no transactions of its own",
				line.Code, strings.Join(ops, ", "))
		}
	}

	fromMonth, toMonth, err := periodMonths(period, req.Granularity)
	if err != nil {
		return Selection{}, err
	}

	// Clipped to what was asked for. This is the whole reason the report and
	// the drill-down share a function rather than each deriving dates: the two
	// agreeing on what "2026-Q1" means is not enough if they disagree on
	// whether January was in the request.
	if fromMonth < req.From {
		fromMonth = req.From
	}
	if toMonth > req.To {
		toMonth = req.To
	}
	if fromMonth > toMonth {
		return Selection{}, fmt.Errorf(
			"report: period %s lies outside the range %s..%s", period, req.From, req.To)
	}

	from, to, err := monthRange(fromMonth, toMonth)
	if err != nil {
		return Selection{}, err
	}
	return Selection{
		EntityID: req.EntityID,
		Basis:    req.Basis,
		From:     from,
		To:       to,
		Line:     line,
	}, nil
}

// periodMonths is the inverse of Granularity.Label: the first and last month a
// column covers, before clipping.
func periodMonths(period string, g Granularity) (from, to string, err error) {
	switch g {
	case Monthly:
		if _, _, err := parseMonth(period); err != nil {
			return "", "", err
		}
		return period, period, nil

	case Quarterly:
		var y, q int
		if _, err := fmt.Sscanf(period, "%4d-Q%1d", &y, &q); err != nil || q < 1 || q > 4 ||
			len(period) != len("2006-Q1") {
			return "", "", fmt.Errorf("report: %q is not a quarter (YYYY-Qn)", period)
		}
		return fmt.Sprintf("%04d-%02d", y, (q-1)*3+1), fmt.Sprintf("%04d-%02d", y, q*3), nil

	case Yearly:
		var y int
		if _, err := fmt.Sscanf(period, "%4d", &y); err != nil || len(period) != len("2006") {
			return "", "", fmt.Errorf("report: %q is not a year (YYYY)", period)
		}
		return fmt.Sprintf("%04d-01", y), fmt.Sprintf("%04d-12", y), nil

	default:
		return "", "", fmt.Errorf("report: %q is not a granularity", g)
	}
}

// monthRange turns two months into the closed interval of dates they span. The
// end is the last day of its month, found by stepping back a day from the first
// of the next: February has three lengths and none of them belong in a constant
// here.
//
// Used by the report's own read and by every drill-down, which is the point --
// an off-by-one day at the edge of a month is invisible in both until they are
// compared, and comparing them is the test this change exists for.
func monthRange(from, to string) (time.Time, time.Time, error) {
	fy, fm, err := parseMonth(from)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	ty, tm, err := parseMonth(to)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if from > to {
		return time.Time{}, time.Time{}, fmt.Errorf("report: period %s is after %s", from, to)
	}

	start := time.Date(fy, time.Month(fm), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(ty, time.Month(tm), 1, 0, 0, 0, 0, time.UTC).
		AddDate(0, 1, 0).AddDate(0, 0, -1)
	return start, end, nil
}

// Cursor is a position in a drill-down, and it is a position rather than an
// offset on purpose.
//
// A drill-down can be thousands of rows and, unlike the review queue, nothing is
// emptying it while somebody reads. (booked_on, id) is stable, unique and
// already indexed, so a cursor over it neither skips a row nor repeats one. The
// review queue uses an offset for the opposite reason: its list shrinks as it is
// worked, so a cursor points into a set that no longer exists.
type Cursor struct {
	BookedOn time.Time
	ID       uuid.UUID
}

// String is the wire form: a date and an id, which is exactly what it is. Not
// encoded, because there is nothing here a client may not know -- it is
// addressing its own rows -- and an opaque cursor would be state to explain
// rather than state to keep.
func (c Cursor) String() string {
	if c.ID == uuid.Nil {
		return ""
	}
	return c.BookedOn.Format("2006-01-02") + ":" + c.ID.String()
}

// ParseCursor reads it back. An empty string is the start of the list and not
// an error: a client asking for the first page has nothing to send.
func ParseCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	date, id, ok := strings.Cut(s, ":")
	if !ok {
		return Cursor{}, fmt.Errorf("report: %q is not a cursor", s)
	}
	on, err := time.Parse("2006-01-02", date)
	if err != nil {
		return Cursor{}, fmt.Errorf("report: cursor date %q: %w", date, err)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return Cursor{}, fmt.Errorf("report: cursor id %q: %w", id, err)
	}
	return Cursor{BookedOn: on, ID: parsed}, nil
}
