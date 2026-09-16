package report_test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MyauDev/vekst/core/internal/normalize"
	"github.com/MyauDev/vekst/core/internal/report"
)

func (f *fixture) request(basis report.Basis) report.Request {
	return report.Request{
		EntityID:    f.entity,
		From:        "2026-03",
		To:          "2026-03",
		Granularity: report.Monthly,
		Basis:       basis,
	}
}

func (f *fixture) open(t *testing.T, req report.Request, period string, line report.LineRef) report.DrillDown {
	t.Helper()
	d, err := f.svc.LineTransactions(context.Background(), f.owner, f.orgID, req, period, line, report.Cursor{}, 0)
	if err != nil {
		t.Fatalf("opening %s in %s: %v", line, period, err)
	}
	return d
}

// ---------------------------------------------------------------------------
// Task 5.1 -- the test this change exists for
// ---------------------------------------------------------------------------

// Every non-zero cell of a report, opened, adds up to the cell.
//
// A drill-down that returns rows summing to something other than the figure
// they were opened from is worse than no drill-down: it makes a correct report
// look wrong, or -- the case that matters -- a wrong report look checked. The
// figure and its rows come from different queries with different shapes, so
// nothing but this comparison can say they select the same set.
//
// Three numbers are compared, not two: the report's figure, the drill-down's
// own total, and the sum of the rows it actually returned. Any two of them
// disagreeing names which of the three code paths drifted.
//
// The signs are the second half of the assertion. `Compute` inverts a cost
// section so an owner reads "OPEX 500.00" rather than "OPEX -500.00"; the store
// holds -50000 and the drill-down returns the store's signs. A test that
// compared absolute values would pass while the page printed revenue as a cost.
func TestEveryCellAddsUpToItsDrillDown(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		txn{key: "tax:acme", amount: 1_200_00, code: revenueCode},
		txn{key: "tax:parts", amount: -300_00, code: costOfSalesCode},
		txn{key: "tax:bank", amount: -40_00, code: opexCode},
		txn{key: "tax:rent", amount: -110_00, code: opexCode},
		txn{key: "tax:wages", amount: -500_00, code: payrollCode},
		txn{key: "tax:lathe", amount: -900_00, code: capexCode},
		txn{key: "tax:mystery", amount: -70_00},
		txn{key: "tax:invoice", amount: 1_200_00, code: revenueCode, kind: "ledger"},
	)

	req := f.request(report.BasisBank)
	r := f.pnl(t, report.BasisBank)

	var opened int
	for _, l := range r.Lines {
		for i, period := range r.Spec.Periods {
			figure := l.ByPeriod[i]

			if l.Computed {
				// A computed line has operands, not transactions. Task 5.2
				// covers what it answers with; here it is only asserted that
				// it does not answer with rows, because rows would be a set
				// somebody invented.
				d := f.open(t, req, period, report.SectionLine(l.Code))
				if d.Kind != report.AnswerOperands {
					t.Errorf("%s answered with %s, want operands", l.Code, d.Kind)
				}
				continue
			}
			if figure.Amount.MinorUnits == 0 {
				continue
			}
			opened++

			d := f.open(t, req, period, report.SectionLine(l.Code))

			// The report's figure is in reading signs; the drill-down is in
			// the store's. NET SALES is money in and is the one line where the
			// two agree.
			want := figure.Amount.MinorUnits
			if l.Code != report.NetSales {
				want = -want
			}

			if d.Total.MinorUnits != want {
				t.Errorf("%s %s: the figure is %d and the cell totals %d",
					l.Code, period, want, d.Total.MinorUnits)
			}
			if got := sumRows(d.Rows); got != want {
				t.Errorf("%s %s: the figure is %d and its %d rows sum to %d",
					l.Code, period, want, len(d.Rows), got)
			}
			if d.RowCount != int64(len(d.Rows)) {
				t.Errorf("%s %s: row_count %d, %d rows returned",
					l.Code, period, d.RowCount, len(d.Rows))
			}
			if d.Total.CurrencyCode != r.Spec.BaseCurrency {
				t.Errorf("%s %s: total is in %q, the report is in %q",
					l.Code, period, d.Total.CurrencyCode, r.Spec.BaseCurrency)
			}
		}
	}

	// And the four buckets, which are not lines and are not inverted.
	for _, b := range report.BucketOrder {
		for i, period := range r.Spec.Periods {
			want := r.Buckets[b][i].MinorUnits
			if want == 0 {
				continue
			}
			opened++

			d := f.open(t, req, period, report.BucketLine(b))
			if d.Total.MinorUnits != want {
				t.Errorf("bucket %s %s: the figure is %d and the cell totals %d",
					b, period, want, d.Total.MinorUnits)
			}
			if got := sumRows(d.Rows); got != want {
				t.Errorf("bucket %s %s: the figure is %d and its %d rows sum to %d",
					b, period, want, len(d.Rows), got)
			}
		}
	}

	// A fixture that stopped producing non-zero cells would make every
	// assertion above vacuous while still passing.
	if opened < 6 {
		t.Fatalf("only %d cells had anything in them; the fixture stopped exercising this", opened)
	}
}

// The same property where it is most likely to break: a quarterly report whose
// range starts mid-quarter.
//
// The column is labelled 2026-Q1 and covers February and March, because that is
// what was asked for. A drill-down deriving its own dates from the label would
// read January too, and return rows the figure never counted -- which is
// exactly why the two share SelectionFor rather than each computing a range.
func TestAClippedPeriodOpensToTheRowsItCounted(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		txn{key: "tax:jan", bookedOn: "2026-01-20", amount: 100_00, code: revenueCode},
		txn{key: "tax:feb", bookedOn: "2026-02-10", amount: 200_00, code: revenueCode},
		txn{key: "tax:mar", bookedOn: "2026-03-10", amount: 400_00, code: revenueCode},
	)

	req := report.Request{
		EntityID: f.entity, From: "2026-02", To: "2026-03",
		Granularity: report.Quarterly, Basis: report.BasisBank,
	}
	r, err := f.svc.ManagementPNL(context.Background(), f.owner, f.orgID, req)
	if err != nil {
		t.Fatalf("ManagementPNL: %v", err)
	}
	if want := []string{"2026-Q1"}; !reflect.DeepEqual(r.Spec.Periods, want) {
		t.Fatalf("periods = %v, want %v", r.Spec.Periods, want)
	}
	if got := lineTotal(t, r, report.NetSales); got.MinorUnits != 600_00 {
		t.Fatalf("NET SALES = %d, want 60000: January is not in the requested range",
			got.MinorUnits)
	}

	d := f.open(t, req, "2026-Q1", report.SectionLine(report.NetSales))
	if d.Total.MinorUnits != 600_00 {
		t.Errorf("the cell totals %d, want 60000 -- the drill-down read the whole quarter "+
			"rather than the part of it that was asked for", d.Total.MinorUnits)
	}
	if len(d.Rows) != 2 {
		t.Errorf("%d rows, want 2", len(d.Rows))
	}
}

// ---------------------------------------------------------------------------
// Task 5.2 -- a computed line opens to its operands
// ---------------------------------------------------------------------------

func TestAComputedLineReturnsOperandsAndEachOpens(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		txn{key: "tax:sales", amount: 1_000_00, code: revenueCode},
		txn{key: "tax:parts", amount: -250_00, code: costOfSalesCode},
	)
	req := f.request(report.BasisBank)

	gm := f.open(t, req, "2026-03", report.SectionLine(report.GM))
	if gm.Kind != report.AnswerOperands {
		t.Fatalf("GM answered with %s, want operands", gm.Kind)
	}
	if len(gm.Rows) != 0 {
		t.Errorf("GM returned %d transactions; it has none of its own, and the set it "+
			"would have to invent is what a reader would misread as a category", len(gm.Rows))
	}

	want := []report.Operand{
		{Code: report.NetSales, Label: "NET SALES"},
		{Code: report.CS, Label: "CS", Subtracted: true},
	}
	if !reflect.DeepEqual(gm.Operands, want) {
		t.Fatalf("GM operands = %+v, want %+v", gm.Operands, want)
	}

	// And each of them opens to transactions in turn.
	for _, o := range gm.Operands {
		d := f.open(t, req, "2026-03", report.SectionLine(o.Code))
		if d.Kind != report.AnswerTransactions {
			t.Errorf("%s answered with %s, want transactions", o.Code, d.Kind)
		}
		if len(d.Rows) != 1 {
			t.Errorf("%s returned %d rows, want 1", o.Code, len(d.Rows))
		}
	}

	// The chain's own arithmetic, checked through the operands: GM is the
	// revenue less the cost, and the operands are what the drill-down says it
	// is made of.
	netSales := f.open(t, req, "2026-03", report.SectionLine(report.NetSales))
	cs := f.open(t, req, "2026-03", report.SectionLine(report.CS))
	if got := netSales.Total.MinorUnits + cs.Total.MinorUnits; got != 750_00 {
		t.Errorf("the operands sum to %d, want 75000 -- a cost is negative in the store, "+
			"so 'NET SALES - CS' is an addition here", got)
	}
}

// ---------------------------------------------------------------------------
// Task 5.3 -- isolation
// ---------------------------------------------------------------------------

func TestOneOrganisationCannotOpenAnothersCell(t *testing.T) {
	a := newFixture(t)
	b := newFixture(t)

	a.insert(t, txn{key: "tax:a", amount: 100_00, code: revenueCode})
	b.insert(t, txn{key: "tax:b", amount: 900_00, code: revenueCode})

	// B's entity, asked for by A: empty, and the same emptiness as an entity
	// that never existed. Anything else makes this a membership oracle with a
	// drill-down attached.
	for name, entity := range map[string]uuid.UUID{
		"another org's entity": b.entity,
		"no such entity":       uuid.New(),
	} {
		req := a.request(report.BasisBank)
		req.EntityID = entity

		d, err := a.svc.LineTransactions(context.Background(), a.owner, a.orgID, req,
			"2026-03", report.SectionLine(report.NetSales), report.Cursor{}, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(d.Rows) != 0 || d.Total.MinorUnits != 0 || d.RowCount != 0 {
			t.Errorf("%s: %d rows totalling %d, want an empty cell",
				name, len(d.Rows), d.Total.MinorUnits)
		}
	}

	// A non-member is refused, not answered with nothing.
	stranger := seedUser(t, a.db)
	_, err := a.svc.LineTransactions(context.Background(), stranger, a.orgID, a.request(report.BasisBank),
		"2026-03", report.SectionLine(report.NetSales), report.Cursor{}, 0)
	if got := codeOf(err); got != report.CodeForbidden {
		t.Errorf("a non-member got %q, want %q", got, report.CodeForbidden)
	}
}

// ---------------------------------------------------------------------------
// Task 5.4 -- why a row is on the line it is on
// ---------------------------------------------------------------------------

// Layer, evidence, confidence and decider come back as they were stored.
//
// These four are the whole point of opening a figure. "Because a Belarusian
// country rule matched this wording" and "because you told us in March" are
// different claims about the same money, and a reviewer trusts them
// differently -- which they cannot do if the drill-down flattens both into "it
// is classified".
func TestEveryRowSaysWhyItIsOnThatLine(t *testing.T) {
	f := newFixture(t)
	decider := f.owner
	f.insert(t,
		txn{key: "tax:rule", amount: -100_00, code: opexCode,
			layer: "L1", evidence: "rule:by-priorbank-commission"},
		txn{key: "tax:person", amount: -200_00, code: opexCode,
			layer: "human", evidence: "categorised", decidedBy: decider},
		txn{key: "tax:code", amount: -300_00, code: opexCode,
			layer: "L0.5", evidence: "knp:332"},
	)

	d := f.open(t, f.request(report.BasisBank), "2026-03", report.SectionLine(report.OPEX))
	if len(d.Rows) != 3 {
		t.Fatalf("%d rows, want 3", len(d.Rows))
	}

	byLayer := map[string]report.DrillRow{}
	for _, r := range d.Rows {
		byLayer[r.EngineLayer] = r
	}

	rule, ok := byLayer["L1"]
	if !ok {
		t.Fatalf("no L1 row among %v", byLayer)
	}
	if rule.Evidence != "rule:by-priorbank-commission" {
		t.Errorf("evidence = %q, want the rule's own scope", rule.Evidence)
	}
	if !rule.HasConfidence || !closeTo(rule.Confidence, 0.95) {
		t.Errorf("confidence = %v (present %v), want 0.95", rule.Confidence, rule.HasConfidence)
	}
	if rule.DecidedBy != uuid.Nil {
		t.Error("a rule named a decider; a machine decision has none")
	}
	if rule.CategoryCode != opexCode {
		t.Errorf("category = %q, want %q", rule.CategoryCode, opexCode)
	}
	if rule.CategoryName == "" {
		t.Error("the category has no name; a code alone is not something a person reads")
	}

	person, ok := byLayer["human"]
	if !ok {
		t.Fatalf("no human row among %v", byLayer)
	}
	if person.DecidedBy != decider {
		t.Errorf("decided_by = %s, want %s -- a human decision names who made it",
			person.DecidedBy, decider)
	}
	if !person.HasConfidence || !closeTo(person.Confidence, 1) {
		t.Errorf("a person's confidence = %v, want 1", person.Confidence)
	}

	code, ok := byLayer["L0.5"]
	if !ok {
		t.Fatalf("no L0.5 row among %v", byLayer)
	}
	if !closeTo(code.Confidence, 0.99) {
		t.Errorf("a regulated code's confidence = %v, want 0.99", code.Confidence)
	}

	// Provenance, on every row: the batch says which file, the line says where
	// in it.
	for _, r := range d.Rows {
		if r.BatchID == uuid.Nil {
			t.Errorf("row %s names no batch", r.ID)
		}
		if r.LineNo <= 0 {
			t.Errorf("row %s claims line %d", r.ID, r.LineNo)
		}
	}
}

// An unclassified row carries no layer, no evidence and no confidence -- and
// that absence is the answer to "why is this here", not a gap in it.
func TestAnUnclassifiedRowClaimsNothing(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "tax:mystery", amount: -80_00})

	d := f.open(t, f.request(report.BasisBank), "2026-03",
		report.BucketLine(report.BucketUnclassified))
	if len(d.Rows) != 1 {
		t.Fatalf("%d rows, want 1", len(d.Rows))
	}
	r := d.Rows[0]
	if r.EngineLayer != "" || r.Evidence != "" || r.CategoryCode != "" {
		t.Errorf("an unclassified row claims layer %q, evidence %q, category %q",
			r.EngineLayer, r.Evidence, r.CategoryCode)
	}
	if r.HasConfidence {
		t.Errorf("an unclassified row reports confidence %v; there is nothing to be "+
			"confident about, and a zero would read as certainty of the opposite", r.Confidence)
	}
	if r.CounterpartyRaw == "" {
		t.Error("the counterparty is empty; it is the only thing a person has to go on here")
	}
}

// ---------------------------------------------------------------------------
// Task 5.5 -- a retraction moves the row, it does not hide it
// ---------------------------------------------------------------------------

func TestARetractedRowOpensInTheUnclassifiedBucket(t *testing.T) {
	f := newFixture(t)
	ids := f.insert(t, txn{key: "tax:taken-back", amount: -140_00, code: opexCode})

	req := f.request(report.BasisBank)
	if d := f.open(t, req, "2026-03", report.SectionLine(report.OPEX)); len(d.Rows) != 1 {
		t.Fatalf("before the retraction OPEX has %d rows, want 1", len(d.Rows))
	}

	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE classifications SET retracted_at = now(), retracted_by = $2
			 WHERE transaction_id = $1`,
			pgtype.UUID{Bytes: ids[0], Valid: true},
			pgtype.UUID{Bytes: f.owner, Valid: true})
		return err
	})
	if err != nil {
		t.Fatalf("retracting: %v", err)
	}

	if d := f.open(t, req, "2026-03", report.SectionLine(report.OPEX)); len(d.Rows) != 0 {
		t.Errorf("OPEX still opens to %d rows after the retraction", len(d.Rows))
	}
	d := f.open(t, req, "2026-03", report.BucketLine(report.BucketUnclassified))
	if len(d.Rows) != 1 || d.Rows[0].ID != ids[0] {
		t.Fatalf("the retracted row is not in the unclassified bucket: %d rows", len(d.Rows))
	}
	if d.Total.MinorUnits != -140_00 {
		t.Errorf("the bucket totals %d, want -14000", d.Total.MinorUnits)
	}
}

// ---------------------------------------------------------------------------
// Task 5.6 -- line_no reaches the drill-down
// ---------------------------------------------------------------------------

// The other half of the round trip. `core/internal/ingest` asserts the number
// survives the persist boundary and indexes the committed fixture it claims;
// this asserts it survives the read, on the row a customer is actually looking
// at.
func TestTheLineNumberReachesTheDrillDown(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		txn{key: "tax:one", amount: 100_00, code: revenueCode},
		txn{key: "tax:two", amount: 200_00, code: revenueCode},
		txn{key: "tax:three", amount: 300_00, code: revenueCode},
	)

	d := f.open(t, f.request(report.BasisBank), "2026-03", report.SectionLine(report.NetSales))
	if len(d.Rows) != 3 {
		t.Fatalf("%d rows, want 3", len(d.Rows))
	}
	seen := map[int32]bool{}
	for _, r := range d.Rows {
		if r.LineNo <= 0 {
			t.Errorf("row %s claims line %d", r.ID, r.LineNo)
		}
		if seen[r.LineNo] {
			t.Errorf("line %d appears twice; a line number that does not identify a line "+
				"is not provenance", r.LineNo)
		}
		seen[r.LineNo] = true
		if r.BatchID != f.bankBatch {
			t.Errorf("row %s names batch %s, want %s", r.ID, r.BatchID, f.bankBatch)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 5.7 -- paging
// ---------------------------------------------------------------------------

// A cursor over a thousand rows neither skips one nor repeats one.
//
// Paged by (booked_on, id) rather than by an offset, and the fixture is built
// to punish the difference: every row shares one booking date, so the date
// alone orders nothing and the tiebreak is doing all the work. An offset would
// pass this test and fail the moment a row were inserted mid-read; a cursor on
// the date alone would skip or repeat here.
func TestACursorOverAThousandRowsNeitherSkipsNorRepeats(t *testing.T) {
	const total = 1000
	f := newFixture(t)
	f.bulkInsert(t, total, "2026-03-15", revenueCode)

	req := f.request(report.BasisBank)
	line := report.SectionLine(report.NetSales)

	seen := map[uuid.UUID]bool{}
	var pages int
	var cursor report.Cursor
	var summed int64
	for {
		d, err := f.svc.LineTransactions(context.Background(), f.owner, f.orgID, req,
			"2026-03", line, cursor, 100)
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		pages++
		for _, r := range d.Rows {
			if seen[r.ID] {
				t.Fatalf("row %s came back twice, on page %d", r.ID, pages)
			}
			seen[r.ID] = true
			summed += r.Amount.MinorUnits
		}
		if d.NextCursor == "" {
			break
		}
		cursor, err = report.ParseCursor(d.NextCursor)
		if err != nil {
			t.Fatalf("parsing the cursor the server just produced: %v", err)
		}
		if pages > total {
			t.Fatal("the cursor never reached the end")
		}
	}

	if len(seen) != total {
		t.Errorf("%d distinct rows over %d pages, want %d", len(seen), pages, total)
	}
	if pages != total/100 {
		t.Errorf("%d pages, want %d -- the last page must not claim a next cursor",
			pages, total/100)
	}

	// And the pages add up to the cell, which is the property that matters:
	// paging is only correct if it partitions.
	d := f.open(t, req, "2026-03", line)
	if d.Total.MinorUnits != summed {
		t.Errorf("the pages sum to %d and the cell totals %d", summed, d.Total.MinorUnits)
	}
}

// ---------------------------------------------------------------------------
// Task 5.8 -- the reconciliation strip
// ---------------------------------------------------------------------------

// opening + in - out - transfers = closing, over three months, with a transfer
// pair in the middle one.
//
// The identity is a comparison between two ways of counting the same rows: the
// three movements come from one aggregation with three FILTER clauses, the two
// balances from a different sum over a different predicate. A row either drops
// or is counted twice, and it breaks.
func TestTheReconciliationStripBalances(t *testing.T) {
	f := newFixture(t)
	ids := f.insert(t,
		txn{key: "tax:before", bookedOn: "2026-01-10", amount: 500_00, code: revenueCode},
		txn{key: "tax:in", bookedOn: "2026-02-10", amount: 300_00, code: revenueCode},
		txn{key: "tax:out", bookedOn: "2026-02-20", amount: -120_00, code: opexCode},
		// Two legs of one movement between the organisation's own accounts.
		txn{key: "tax:move-out", bookedOn: "2026-03-05", amount: -200_00},
		txn{key: "tax:move-in", bookedOn: "2026-03-05", amount: 200_00},
	)
	f.pairTransfer(t, ids[3], ids[4])

	req := report.Request{
		EntityID: f.entity, From: "2026-02", To: "2026-04",
		Granularity: report.Monthly, Basis: report.BasisBank,
	}
	r, err := f.svc.ManagementPNL(context.Background(), f.owner, f.orgID, req)
	if err != nil {
		t.Fatalf("ManagementPNL: %v", err)
	}
	if len(r.Reconciliation) != 3 {
		t.Fatalf("%d strips, want one per period", len(r.Reconciliation))
	}

	for _, s := range r.Reconciliation {
		if !s.Balances {
			t.Errorf("%s: %d + %d - %d - %d <> %d", s.Period,
				s.Opening.MinorUnits, s.In.MinorUnits, s.Out.MinorUnits,
				s.Transfers.MinorUnits, s.Closing.MinorUnits)
		}
		if !s.Derived {
			t.Errorf("%s: the strip does not say its balances are derived. Nothing stores "+
				"what the statement itself declared yet, and a client rendering this as "+
				"'reconciled with your bank' would claim more than it can", s.Period)
		}
		if s.Opening.CurrencyCode == "" || s.Closing.CurrencyCode == "" {
			t.Errorf("%s: a balance with no code beside it is not money", s.Period)
		}
	}

	feb, mar := r.Reconciliation[0], r.Reconciliation[1]

	// February opens where January left off, and the row booked in January is
	// outside the range but not outside the business.
	if feb.Opening.MinorUnits != 500_00 {
		t.Errorf("February opens at %d, want 50000 from the January row", feb.Opening.MinorUnits)
	}
	if feb.In.MinorUnits != 300_00 || feb.Out.MinorUnits != 120_00 {
		t.Errorf("February in/out = %d/%d, want 30000/12000 as positive magnitudes",
			feb.In.MinorUnits, feb.Out.MinorUnits)
	}
	if feb.Closing.MinorUnits != mar.Opening.MinorUnits {
		t.Errorf("February closes at %d and March opens at %d; a ledger is continuous",
			feb.Closing.MinorUnits, mar.Opening.MinorUnits)
	}

	// The transfer's two legs are counted apart from in and out. Both are
	// inside this entity, so they net to zero -- and the count is what makes
	// that zero legible: "0 across 2 rows" and "0 across none" are different
	// facts about a business.
	if mar.TransferRowCount != 2 {
		t.Errorf("March saw %d transfer rows, want 2", mar.TransferRowCount)
	}
	if mar.Transfers.MinorUnits != 0 {
		t.Errorf("March transfers = %d; both legs are in this entity and net to nothing",
			mar.Transfers.MinorUnits)
	}
	if mar.In.MinorUnits != 0 || mar.Out.MinorUnits != 0 {
		t.Errorf("March in/out = %d/%d; a transfer is the organisation moving its own "+
			"money, and calling it revenue in one account and an expense in another is how "+
			"a business appears to trade with itself", mar.In.MinorUnits, mar.Out.MinorUnits)
	}
}

// The identity, on the shapes most likely to break it.
//
// It can only be broken by a bug in the code that computes it -- which is the
// point, and also the difficulty with testing it: there is no fixture that
// makes a correct implementation disagree with itself. So the test does the
// next best thing and puts the two aggregations under the cases where they read
// the same rows differently. A transfer with one leg inside the period and one
// outside is the sharpest: the movement query excludes the leg it can see, and
// the balance query includes it, so the transfers term is the only thing that
// can reconcile them. A dismissed pair is the mirror image -- the membership row
// is still there and the legs are ordinary money again.
//
// An empty month is here for the opposite reason: every term is zero, which a
// broken implementation can still get wrong and a hardcoded `true` gets right.
func TestTheIdentityHoldsOnTheAwkwardShapes(t *testing.T) {
	f := newFixture(t)
	ids := f.insert(t,
		// A pair whose legs fall in different months.
		txn{key: "tax:leg-out", bookedOn: "2026-03-28", amount: -400_00},
		txn{key: "tax:leg-in", bookedOn: "2026-04-02", amount: 400_00},
		// A pair somebody dismissed: not a transfer any more.
		txn{key: "tax:not-a-move-out", bookedOn: "2026-03-10", amount: -90_00},
		txn{key: "tax:not-a-move-in", bookedOn: "2026-03-11", amount: 90_00},
		// Ordinary trade, so the months are not all zero.
		txn{key: "tax:sale", bookedOn: "2026-04-15", amount: 600_00, code: revenueCode},
	)
	f.pairTransfer(t, ids[0], ids[1])
	dismissed := f.pairTransfer(t, ids[2], ids[3])
	f.dismissTransfer(t, dismissed)

	req := report.Request{
		EntityID: f.entity, From: "2026-03", To: "2026-05",
		Granularity: report.Monthly, Basis: report.BasisBank,
	}
	r, err := f.svc.ManagementPNL(context.Background(), f.owner, f.orgID, req)
	if err != nil {
		t.Fatalf("ManagementPNL: %v", err)
	}
	if len(r.Reconciliation) != 3 {
		t.Fatalf("%d strips, want 3", len(r.Reconciliation))
	}

	for _, s := range r.Reconciliation {
		if !s.Balances {
			t.Errorf("%s: %d + %d - %d - %d <> %d", s.Period,
				s.Opening.MinorUnits, s.In.MinorUnits, s.Out.MinorUnits,
				s.Transfers.MinorUnits, s.Closing.MinorUnits)
		}
	}
	march, april, may := r.Reconciliation[0], r.Reconciliation[1], r.Reconciliation[2]

	// March sees one leg of the split pair, and it is an outflow of 400 that
	// is not an expense.
	if march.TransferRowCount != 1 {
		t.Errorf("March saw %d transfer rows, want 1 -- the other leg is in April",
			march.TransferRowCount)
	}
	if march.Transfers.MinorUnits != 400_00 {
		t.Errorf("March transfers = %d, want 40000 as an outflow", march.Transfers.MinorUnits)
	}
	// The dismissed pair is ordinary money in and out, not a transfer.
	if march.In.MinorUnits != 90_00 || march.Out.MinorUnits != 90_00 {
		t.Errorf("March in/out = %d/%d, want 9000/9000: a dismissed pair is not a transfer, "+
			"and its legs go back to being movement", march.In.MinorUnits, march.Out.MinorUnits)
	}

	if april.TransferRowCount != 1 {
		t.Errorf("April saw %d transfer rows, want 1", april.TransferRowCount)
	}
	if april.Transfers.MinorUnits != -400_00 {
		t.Errorf("April transfers = %d, want -40000: the leg that arrives is an inflow, "+
			"and the term is signed as an outflow", april.Transfers.MinorUnits)
	}

	// May traded nothing, and a month of zeros still has to balance.
	if may.In.MinorUnits != 0 || may.Out.MinorUnits != 0 || may.TransferRowCount != 0 {
		t.Errorf("May is not empty: %+v", may)
	}
	if may.Opening != may.Closing {
		t.Errorf("May opened at %v and closed at %v with no movement", may.Opening, may.Closing)
	}
	// And it opened where April closed: a ledger is continuous across an empty
	// month as much as across a busy one.
	if april.Closing != may.Opening {
		t.Errorf("April closed at %v and May opened at %v", april.Closing, may.Opening)
	}
}

// ---------------------------------------------------------------------------
// Task 5.9 -- a row in another currency
// ---------------------------------------------------------------------------

// The drill-down shows what the statement said; the figure is summed from what
// it converts to. Both, on the same row, because a person checking a 7,000
// figure against a 1,000,000 line in their file needs to see both numbers to
// believe either.
func TestAForeignRowShowsItsOwnAmountAndContributesItsBase(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		// 1,000,000 JPY minor units is a big number and a small amount.
		txn{key: "tax:tokyo", amount: 1_000_000, currency: "JPY", baseAmount: 70_00,
			code: revenueCode},
		txn{key: "tax:oslo", amount: 30_00, code: revenueCode},
	)

	d := f.open(t, f.request(report.BasisBank), "2026-03", report.SectionLine(report.NetSales))
	if len(d.Rows) != 2 {
		t.Fatalf("%d rows, want 2", len(d.Rows))
	}

	var foreign, domestic report.DrillRow
	for _, r := range d.Rows {
		if r.Amount.CurrencyCode == "JPY" {
			foreign = r
		} else {
			domestic = r
		}
	}

	if foreign.Amount.MinorUnits != 1_000_000 {
		t.Errorf("the JPY row shows %d, want the 1000000 the statement said",
			foreign.Amount.MinorUnits)
	}
	if foreign.BaseAmount.CurrencyCode != "NOK" || foreign.BaseAmount.MinorUnits != 70_00 {
		t.Errorf("the JPY row converts to %v, want 7000 NOK", foreign.BaseAmount)
	}
	// "No conversion happened" is a different claim from "converted at a rate
	// of one", and a row already in the base currency makes the first.
	if domestic.BaseAmount.CurrencyCode != "" {
		t.Errorf("a NOK row claims a conversion to %v", domestic.BaseAmount)
	}

	// And the cell is the sum of the base amounts, never of the face values:
	// 1,000,000 + 3,000 would be arithmetic on incompatible units.
	if d.Total.MinorUnits != 100_00 {
		t.Errorf("the cell totals %d, want 10000 -- face values were summed across "+
			"currencies", d.Total.MinorUnits)
	}
}

// ---------------------------------------------------------------------------
// Task 5.10 -- no money field is a float
// ---------------------------------------------------------------------------

func TestNoDrillDownMoneyIsAFloat(t *testing.T) {
	for _, v := range []any{report.DrillRow{}, report.DrillDown{}, report.Reconciliation{}} {
		typ := reflect.TypeOf(v)
		for i := range typ.NumField() {
			field := typ.Field(i)
			switch field.Type.Kind() {
			case reflect.Float32, reflect.Float64:
				// Confidence is a probability and not money; a float is
				// exactly the right type for it, and it is the only one.
				if typ == reflect.TypeOf(report.DrillRow{}) && field.Name == "Confidence" {
					continue
				}
				t.Errorf("%s.%s is a %s. Money is int64 minor units plus an ISO-4217 code, "+
					"never a float, in any language (CLAUDE.md)", typ.Name(), field.Name,
					field.Type.Kind())
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Addressing a cell
// ---------------------------------------------------------------------------

// A cell that does not exist is a bad request, never an empty page. An empty
// drill-down of a figure of 412,000 reads as "these rows went missing", and a
// reader cannot tell that from "you asked for a line this report does not have".
func TestOpeningSomethingThatIsNotACellIsRefused(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "tax:a", amount: 100_00, code: revenueCode})
	req := f.request(report.BasisBank)

	if _, err := report.ParseLineRef("99"); err == nil {
		t.Error("'99' was accepted as a line")
	}
	if _, err := report.ParseLineRef("net sales"); err == nil {
		t.Error("a category name was accepted as a line")
	}

	_, err := f.svc.LineTransactions(context.Background(), f.owner, f.orgID, req,
		"2026-04", report.SectionLine(report.NetSales), report.Cursor{}, 0)
	if got := codeOf(err); got != report.CodeUnknownPeriod {
		t.Errorf("a period outside the report answered %q, want %q", got, report.CodeUnknownPeriod)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// Confidence is the one float in this package, and it makes the round trip
// through numeric(4,3) and back. Comparing two floats for exact equality is a
// bug even when it happens to pass: the column keeps three decimal places, the
// conversion is decimal-to-binary, and 0.95 is not representable in either
// direction. A tolerance far tighter than the column's own precision asserts
// the same thing without asserting the representation.
func closeTo(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

func sumRows(rows []report.DrillRow) int64 {
	var total int64
	for _, r := range rows {
		if r.BaseAmount.CurrencyCode != "" {
			total += r.BaseAmount.MinorUnits
			continue
		}
		total += r.Amount.MinorUnits
	}
	return total
}

// bulkInsert plants n rows in one statement. The paging test needs a thousand
// and does not care what is in them.
func (f *fixture) bulkInsert(t *testing.T, n int, bookedOn, code string) {
	t.Helper()
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO transactions (
				org_id, entity_id, account_id, batch_id, line_no, source_kind,
				booked_on, amount_minor, currency,
				counterparty_raw, counterparty_key, description_raw,
				description_norm, normalize_version, dedup_hash)
			SELECT app_current_org(), $1, a.id, $2, g.i, 'bank',
			       $3::date, 100, 'NOK',
			       'Bulk', 'tax:bulk', 'Bulk', 'bulk', $4, gen_random_uuid()::text
			  FROM generate_series(1, $5::int) AS g(i),
			       LATERAL (SELECT id FROM accounts WHERE entity_id = $1 LIMIT 1) a`,
			pgtype.UUID{Bytes: f.entity, Valid: true},
			pgtype.UUID{Bytes: f.bankBatch, Valid: true},
			bookedOn, normalize.Version, n); err != nil {
			return fmt.Errorf("bulk transactions: %w", err)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO classifications (
				org_id, transaction_id, category_id, engine_layer, confidence,
				taxonomy_version, ruleset_version, engine_version, normalize_version)
			SELECT app_current_org(), t.id, c.id, 'L1', 0.950, $1, $2, 'engine-1', $3
			  FROM transactions t
			  CROSS JOIN (SELECT id FROM categories
			               WHERE taxonomy_version = $1 AND code = $4 AND org_id IS NULL) c
			 WHERE t.counterparty_key = 'tax:bulk'`,
			taxonomyV, rulesetV, normalize.Version, code)
		return err
	})
	if err != nil {
		t.Fatalf("bulk seeding: %v", err)
	}
}

// pairTransfer marks two rows as the two legs of one movement between the
// organisation's own accounts, the way change 2.6's detector would.
func (f *fixture) pairTransfer(t *testing.T, out, in uuid.UUID) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO internal_transfers (org_id, out_txn_id, in_txn_id)
			VALUES (app_current_org(), $1, $2) RETURNING id`,
			pgtype.UUID{Bytes: out, Valid: true},
			pgtype.UUID{Bytes: in, Valid: true}).Scan(&id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO internal_transfer_members (org_id, txn_id, transfer_id, side)
			VALUES (app_current_org(), $1, $3, 'out'), (app_current_org(), $2, $3, 'in')`,
			pgtype.UUID{Bytes: out, Valid: true},
			pgtype.UUID{Bytes: in, Valid: true}, id)
		return err
	})
	if err != nil {
		t.Fatalf("pairing a transfer: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

// dismissTransfer is a person saying "these two are not the same movement".
// The membership rows stay -- the claim was made and is part of the record --
// so the reconciliation has to read the dismissal rather than the membership.
func (f *fixture) dismissTransfer(t *testing.T, id uuid.UUID) {
	t.Helper()
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE internal_transfers SET dismissed_at = now(), dismissed_by = $2
			 WHERE id = $1`,
			pgtype.UUID{Bytes: id, Valid: true},
			pgtype.UUID{Bytes: f.owner, Valid: true})
		return err
	})
	if err != nil {
		t.Fatalf("dismissing a transfer: %v", err)
	}
}
