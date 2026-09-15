package report_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MyauDev/vekst/core/internal/report"
)

// Addressing a cell, without a database.
//
// This is the half of the drill-down that decides *which* rows a figure was
// summed from, and it is a pure function for the same reason the calculation
// is: the report and the drill-down have to agree, and two things agree most
// reliably when they are one thing. The database tests prove the rows come
// back; these prove the address was right before anybody went looking.

func req(from, to string, g report.Granularity) report.Request {
	return report.Request{
		EntityID:    uuid.New(),
		From:        from,
		To:          to,
		Granularity: g,
		Basis:       report.BasisBank,
	}
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// A column's dates, at each granularity, and clipped to what was asked for.
//
// The clipping is the case that matters. A quarterly report starting in
// February has a Q1 column covering February and March; a drill-down that read
// the whole quarter would return January's rows, which the figure never
// counted, and the page would look like the report was wrong.
func TestSelectionForCoversTheColumnAndNoMore(t *testing.T) {
	for name, c := range map[string]struct {
		req      report.Request
		period   string
		from, to time.Time
	}{
		"a month": {
			req("2026-01", "2026-12", report.Monthly), "2026-02",
			day(2026, time.February, 1), day(2026, time.February, 28),
		},
		"a leap February": {
			req("2024-01", "2024-12", report.Monthly), "2024-02",
			day(2024, time.February, 1), day(2024, time.February, 29),
		},
		"a whole quarter": {
			req("2026-01", "2026-12", report.Quarterly), "2026-Q1",
			day(2026, time.January, 1), day(2026, time.March, 31),
		},
		"the last quarter": {
			req("2026-01", "2026-12", report.Quarterly), "2026-Q4",
			day(2026, time.October, 1), day(2026, time.December, 31),
		},
		"a quarter clipped at the start": {
			req("2026-02", "2026-12", report.Quarterly), "2026-Q1",
			day(2026, time.February, 1), day(2026, time.March, 31),
		},
		"a quarter clipped at both ends": {
			req("2026-02", "2026-02", report.Quarterly), "2026-Q1",
			day(2026, time.February, 1), day(2026, time.February, 28),
		},
		"a year": {
			req("2025-01", "2026-12", report.Yearly), "2025",
			day(2025, time.January, 1), day(2025, time.December, 31),
		},
		"a year clipped at the end": {
			req("2026-01", "2026-05", report.Yearly), "2026",
			day(2026, time.January, 1), day(2026, time.May, 31),
		},
	} {
		t.Run(name, func(t *testing.T) {
			sel, err := report.SelectionFor(c.req, c.period, report.SectionLine(report.NetSales))
			if err != nil {
				t.Fatalf("SelectionFor: %v", err)
			}
			if !sel.From.Equal(c.from) || !sel.To.Equal(c.to) {
				t.Errorf("%s covers %s..%s, want %s..%s", c.period,
					sel.From.Format("2006-01-02"), sel.To.Format("2006-01-02"),
					c.from.Format("2006-01-02"), c.to.Format("2006-01-02"))
			}
			if sel.EntityID != c.req.EntityID || sel.Basis != c.req.Basis {
				t.Errorf("the selection lost the entity or the basis: %+v", sel)
			}
		})
	}
}

// Every column of a report covers its own months and no other's, and together
// they cover the whole range. A gap loses transactions from every drill-down at
// once; an overlap counts one twice.
func TestTheColumnsPartitionTheRange(t *testing.T) {
	for _, g := range []report.Granularity{report.Monthly, report.Quarterly, report.Yearly} {
		r := req("2025-11", "2027-02", g)
		periods, err := report.Periods(r.From, r.To, g)
		if err != nil {
			t.Fatalf("%s: %v", g, err)
		}

		var prev report.Selection
		for i, p := range periods {
			sel, err := report.SelectionFor(r, p, report.SectionLine(report.NetSales))
			if err != nil {
				t.Fatalf("%s %s: %v", g, p, err)
			}
			if sel.To.Before(sel.From) {
				t.Errorf("%s %s runs backwards", g, p)
			}
			if i == 0 {
				if !sel.From.Equal(day(2025, time.November, 1)) {
					t.Errorf("%s: the first column opens at %s, not at the range's own start",
						g, sel.From.Format("2006-01-02"))
				}
			} else if want := prev.To.AddDate(0, 0, 1); !sel.From.Equal(want) {
				t.Errorf("%s: %s opens at %s, and the column before it closed at %s -- "+
					"a day between two columns is a day no drill-down can reach", g, p,
					sel.From.Format("2006-01-02"), prev.To.Format("2006-01-02"))
			}
			prev = sel
		}
		if !prev.To.Equal(day(2027, time.February, 28)) {
			t.Errorf("%s: the last column closes at %s, not at the range's own end",
				g, prev.To.Format("2006-01-02"))
		}
	}
}

func TestSelectionForRefusals(t *testing.T) {
	// A computed line has operands, not transactions. Returning an empty
	// selection instead would let a caller that skipped Operands invent a set
	// in silence.
	for _, code := range []string{report.GM, report.NM, report.CM, report.IBT, report.NI} {
		if _, err := report.SelectionFor(req("2026-01", "2026-12", report.Monthly),
			"2026-01", report.SectionLine(code)); err == nil {
			t.Errorf("%s produced a selection; it is computed and has no rows of its own", code)
		}
	}

	// A period the report does not have.
	if _, err := report.SelectionFor(req("2026-02", "2026-03", report.Monthly),
		"2026-09", report.SectionLine(report.NetSales)); err == nil {
		t.Error("a period outside the range produced a selection")
	}

	// Labels that do not parse at the granularity they are read under.
	for _, c := range []struct {
		g      report.Granularity
		period string
	}{
		{report.Monthly, "2026-Q1"},
		{report.Monthly, "2026"},
		{report.Quarterly, "2026-03"},
		{report.Quarterly, "2026-Q5"},
		{report.Quarterly, "2026-Q0"},
		{report.Yearly, "2026-03"},
		{report.Granularity("fortnight"), "2026-03"},
	} {
		if _, err := report.SelectionFor(req("2026-01", "2026-12", c.g), c.period,
			report.SectionLine(report.NetSales)); err == nil {
			t.Errorf("%s accepted %q as a period", c.g, c.period)
		}
	}
}

// A line is a taxonomy code or a bucket name, and the two vocabularies are
// closed and disjoint -- so no prefix is needed to tell them apart, and
// membership is what is checked rather than shape.
func TestParseLineRef(t *testing.T) {
	for _, code := range append(append([]string{}, report.Order...), report.NonPNLSections...) {
		got, err := report.ParseLineRef(code)
		if err != nil {
			t.Errorf("ParseLineRef(%q): %v", code, err)
			continue
		}
		if got.IsBucket() || got.Code != code || got.String() != code {
			t.Errorf("%q parsed to %+v", code, got)
		}
	}

	for _, b := range report.BucketOrder {
		got, err := report.ParseLineRef(string(b))
		if err != nil {
			t.Errorf("ParseLineRef(%q): %v", b, err)
			continue
		}
		if !got.IsBucket() || got.Bucket != b || got.String() != string(b) {
			t.Errorf("%q parsed to %+v", b, got)
		}
	}

	// A cell that does not exist is a bad request, never an empty page.
	for _, bad := range []string{"", "99", "0101", "NET SALES", "unclassified ", "UNCLASSIFIED"} {
		if _, err := report.ParseLineRef(bad); err == nil {
			t.Errorf("ParseLineRef(%q) was accepted", bad)
		}
	}
}

// A computed line names the operands the chain uses, and a section names none.
func TestOperands(t *testing.T) {
	for _, c := range []struct {
		code string
		want []string
	}{
		{report.GM, []string{report.NetSales, report.CS}},
		{report.NM, []string{report.GM, report.OCS}},
		{report.CM, []string{report.NM, report.OPEX, report.OIE}},
		{report.IBT, []string{report.CM, report.FR}},
		{report.NI, []string{report.IBT, report.CIT}},
	} {
		got := report.Operands(c.code)
		if len(got) != len(c.want) {
			t.Errorf("%s has operands %v, want %v", c.code, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s has operands %v, want %v", c.code, got, c.want)
				break
			}
		}
	}
	for _, code := range append([]string{}, report.Sections...) {
		if ops := report.Operands(code); ops != nil {
			t.Errorf("section %s claims operands %v; it has transactions instead", code, ops)
		}
	}
}

// A cursor is a date and an id, not an opaque token. There is nothing in it a
// client may not know -- it is addressing its own rows -- and an opaque one
// would be state to explain rather than state to keep.
func TestCursorRoundTrips(t *testing.T) {
	want := report.Cursor{BookedOn: day(2026, time.March, 15), ID: uuid.New()}
	got, err := report.ParseCursor(want.String())
	if err != nil {
		t.Fatalf("ParseCursor(%q): %v", want.String(), err)
	}
	if !got.BookedOn.Equal(want.BookedOn) || got.ID != want.ID {
		t.Errorf("round trip produced %+v, want %+v", got, want)
	}

	// The empty cursor is the first page, not a malformed one: a client asking
	// for it has nothing to send.
	if s := (report.Cursor{}).String(); s != "" {
		t.Errorf("the zero cursor renders as %q", s)
	}
	if c, err := report.ParseCursor(""); err != nil || c.ID != uuid.Nil {
		t.Errorf("ParseCursor of the empty string = %+v, %v", c, err)
	}

	for _, bad := range []string{"yesterday", "2026-03-15", "2026-03-15:not-a-uuid",
		"not-a-date:" + uuid.NewString()} {
		if _, err := report.ParseCursor(bad); err == nil {
			t.Errorf("ParseCursor(%q) was accepted", bad)
		}
	}
}

// The backend returns error codes, never sentences: translation is the
// client's. So a report failure's Error() is its code, and the cause it wraps
// stays reachable for a log without reaching a user.
func TestACodedFailureIsItsCode(t *testing.T) {
	err := error(&report.Err{Code: report.CodeBadRange})

	if err.Error() != report.CodeBadRange {
		t.Errorf("Error() = %q, want the code itself", err.Error())
	}
	if errors.Unwrap(err) != nil {
		t.Error("a coded failure with no cause unwrapped to something")
	}

	var coded *report.Err
	if !errors.As(fmt.Errorf("wrapped: %w", &report.Err{Code: report.CodeForbidden}), &coded) ||
		coded.Code != report.CodeForbidden {
		t.Error("a coded failure did not survive being wrapped")
	}
}
