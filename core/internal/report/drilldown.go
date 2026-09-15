package report

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/money"
)

// Error codes for opening a figure.
const (
	CodeUnknownLine    = "report_unknown_line"
	CodeUnknownPeriod  = "report_unknown_period"
	CodeBadCursor      = "report_bad_cursor"
	CodeLineIsComputed = "report_line_is_computed"
)

// Answer says what kind of thing a drill-down came back with, so a screen
// renders a list of rows or a list of lines without guessing.
type Answer string

const (
	AnswerTransactions Answer = "transactions"

	// AnswerOperands is what a computed line gives back. GM has no
	// transactions of its own; it is NET SALES minus CS, and both of those
	// have transactions.
	AnswerOperands Answer = "operands"
)

// DrillRow is one transaction behind a figure, with the four fields that say
// why it is on that line.
type DrillRow struct {
	ID       uuid.UUID
	BookedOn string // YYYY-MM-DD

	// Where it came from. The batch says which file, the line says where in it
	// -- and a customer checking a figure has that file open.
	BatchID     uuid.UUID
	LineNo      int32
	PostingNo   int32
	DocumentRef string

	// What the source said, and the conversion into the organisation's base
	// currency. BaseAmount is zero-valued when the row needed no conversion:
	// "no conversion happened" is a different claim from "converted at one".
	Amount     money.Money
	BaseAmount money.Money

	CounterpartyRaw string
	Description     string
	RegulatedCode   string
	SourceKind      string

	// The live classification, absent on a row in one of the buckets.
	CategoryCode string
	CategoryName string

	// Why this row is on this line. `L0` memory, `L0.5` a regulated code, `L1`
	// a rule, `human` a person -- and the evidence beside it. "Because a
	// Belarusian country rule matched this wording" and "because you told us in
	// March" are different claims about the same figure, and a reviewer trusts
	// them differently.
	EngineLayer   string
	Evidence      string
	Confidence    float64
	HasConfidence bool

	// Set exactly when a person decided.
	DecidedBy uuid.UUID
}

// Operand is one line a computed line is made of.
type Operand struct {
	Code  string
	Label string

	// Subtracted reports whether the formula takes this operand away. It is
	// how the line reads on the page, not how the arithmetic runs: a cost is
	// already negative in the store, so "minus" there is an addition here.
	Subtracted bool
}

// DrillDown is one opened cell.
type DrillDown struct {
	Line   LineRef
	Period string
	Kind   Answer

	Rows     []DrillRow
	Operands []Operand

	// The whole cell, not this page: a reader has to know whether what they
	// are looking at is the answer or the start of it.
	RowCount int64
	Total    money.Money

	// Empty when the page just returned is the last one.
	NextCursor string
}

// Reconciliation is the strip at the foot of the table, for one period.
//
// It is checkable, which is what makes it worth building:
//
//	opening + in - out - transfers = closing
//
// When it does not hold the report says so rather than printing five numbers
// that do not add up.
type Reconciliation struct {
	Period string

	Opening   money.Money
	In        money.Money
	Out       money.Money
	Transfers money.Money
	Closing   money.Money

	// TransferRowCount is what makes a zero legible. Both legs of a pair inside
	// one entity net to nothing, and "0 across 4 rows" and "0 across none" are
	// different facts about a business.
	TransferRowCount int64

	// Derived says the balances were computed from the rows rather than read
	// from what the statement itself declared, which nothing stores yet --
	// change 2.3's balance check is where those arrive. So the identity below
	// checks this system against itself and not against the bank: it catches a
	// row the filters drop or count twice, and it cannot catch a row the import
	// never saw. Saying which is the difference between an informative strip
	// and a check somebody trusts for something it cannot do.
	Derived bool

	// Balances is the identity above. False is a report that says so.
	Balances bool
}

// LineTransactions opens one cell.
//
// The selection is built by SelectionFor -- the same function that clips a
// period to the requested range for the figure itself. That sharing is the
// point: two independently derived date ranges agree until a report starts
// mid-quarter, and then the drill-down quietly returns rows the figure never
// counted.
func (s *Service) LineTransactions(
	ctx context.Context,
	userID, orgID uuid.UUID,
	req Request,
	period string,
	line LineRef,
	cursor Cursor,
	limit int32,
) (DrillDown, error) {
	switch req.Basis {
	case BasisLedger, BasisBank:
	default:
		return DrillDown{}, codeErr(CodeUnknownBasis,
			fmt.Errorf("report: %q is not a basis", req.Basis))
	}
	periods, err := Periods(req.From, req.To, req.Granularity)
	if err != nil {
		return DrillDown{}, codeErr(CodeUnknownGranularity, err)
	}
	if !contains(periods, period) {
		// A period outside the report is a bad request and not an empty
		// result: an empty drill-down of a figure of 412,000 reads as "these
		// rows went missing".
		return DrillDown{}, codeErr(CodeUnknownPeriod,
			fmt.Errorf("report: %q is not a period of this report", period))
	}

	out := DrillDown{Line: line, Period: period, Kind: AnswerTransactions}

	// A computed line answers with the lines it is made of. One extra level of
	// indirection, and the honest one: the set a computed line's "transactions"
	// would have to be -- everything in NET SALES and CS -- is exactly what a
	// reader would misread as a single category.
	if !line.IsBucket() {
		if ops := Operands(line.Code); len(ops) > 0 {
			out.Kind = AnswerOperands
			for _, l := range Chain {
				if l.Code != line.Code {
					continue
				}
				for _, c := range l.Plus {
					out.Operands = append(out.Operands, Operand{Code: c, Label: labelOf(c)})
				}
				for _, c := range l.Minus {
					out.Operands = append(out.Operands, Operand{
						Code: c, Label: labelOf(c), Subtracted: true})
				}
			}
			return out, nil
		}
	}

	sel, err := SelectionFor(req, period, line)
	if err != nil {
		return DrillDown{}, codeErr(CodeUnknownLine, err)
	}

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	org, err := s.bind(ctx, userID, orgID)
	if err != nil {
		return DrillDown{}, err
	}

	err = s.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		baseCcy, err := q.OrganizationBaseCurrency(ctx)
		if err != nil {
			return fmt.Errorf("report: reading the reporting currency: %w", err)
		}

		total, err := q.LineTotal(ctx, gendb.LineTotalParams{
			EntityID:   pgUUID(sel.EntityID),
			FromDate:   pgDate(sel.From),
			ToDate:     pgDate(sel.To),
			Line:       sel.Line.String(),
			SourceKind: string(sel.Basis),
		})
		if err != nil {
			return fmt.Errorf("report: totalling the cell: %w", err)
		}
		out.RowCount = total.RowCount
		out.Total = money.Money{CurrencyCode: baseCcy, MinorUnits: total.AmountMinor}

		// One more than asked for, so "is there another page" is answered by
		// the read rather than guessed from a full one. A page that happens to
		// be exactly the limit is otherwise indistinguishable from the last.
		rows, err := q.LineTransactions(ctx, gendb.LineTransactionsParams{
			EntityID:       pgUUID(sel.EntityID),
			FromDate:       pgDate(sel.From),
			ToDate:         pgDate(sel.To),
			Line:           sel.Line.String(),
			SourceKind:     string(sel.Basis),
			CursorBookedOn: pgtype.Date{Time: cursor.BookedOn, Valid: cursor.ID != uuid.Nil},
			CursorID:       pgtype.UUID{Bytes: cursor.ID, Valid: cursor.ID != uuid.Nil},
			RowLimit:       limit + 1,
		})
		if err != nil {
			return fmt.Errorf("report: reading the cell: %w", err)
		}
		if len(rows) > int(limit) {
			last := rows[limit-1]
			out.NextCursor = Cursor{
				BookedOn: last.BookedOn.Time,
				ID:       uuid.UUID(last.ID.Bytes),
			}.String()
			rows = rows[:limit]
		}

		for _, r := range rows {
			row := DrillRow{
				ID:              uuid.UUID(r.ID.Bytes),
				BookedOn:        r.BookedOn.Time.Format("2006-01-02"),
				BatchID:         uuid.UUID(r.BatchID.Bytes),
				LineNo:          r.LineNo,
				PostingNo:       r.PostingNo,
				DocumentRef:     r.DocumentRef.String,
				Amount:          money.Money{CurrencyCode: r.Currency, MinorUnits: r.AmountMinor},
				CounterpartyRaw: r.CounterpartyRaw,
				Description:     r.DescriptionRaw,
				RegulatedCode:   r.RegulatedCode,
				SourceKind:      r.SourceKind,
				CategoryCode:    r.CategoryCode,
				CategoryName:    r.CategoryName,
				EngineLayer:     r.EngineLayer,
				Evidence:        r.Evidence,
			}
			if r.BaseAmountMinor.Valid && r.BaseCurrency.Valid {
				row.BaseAmount = money.Money{
					CurrencyCode: r.BaseCurrency.String,
					MinorUnits:   r.BaseAmountMinor.Int64,
				}
			}
			if r.Confidence.Valid {
				v, err := r.Confidence.Float64Value()
				if err != nil {
					return fmt.Errorf("report: reading confidence: %w", err)
				}
				row.Confidence, row.HasConfidence = v.Float64, true
			}
			if r.DecidedBy.Valid {
				row.DecidedBy = uuid.UUID(r.DecidedBy.Bytes)
			}
			out.Rows = append(out.Rows, row)
		}
		return nil
	})
	if err != nil {
		return DrillDown{}, err
	}
	return out, nil
}

// reconciliation builds the strip for every period of a report, inside the
// caller's own transaction.
//
// The balances are read at the N+1 boundaries between N periods, so each
// period's closing is the next one's opening by construction -- a continuous
// ledger is what that means, and reading the two separately would let them
// disagree by a row booked exactly on the seam.
//
// What is *not* by construction is the identity. `in`, `out` and `transfers`
// come from one aggregation with three FILTER clauses; the balances come from a
// different sum over a different predicate. So
//
//	opening + in - out - transfers = closing
//
// is a real comparison between two ways of counting the same rows, and a row
// the filters drop -- or count twice -- breaks it. What it cannot check is
// whether the rows match the bank, because nothing stores what the statement
// itself declared; that is change 2.3's balance check, and until it lands the
// strip says `Derived` so nobody reads more into it than it can carry.
func reconciliation(
	ctx context.Context,
	q *gendb.Queries,
	req Request,
	periods []string,
	baseCcy string,
) ([]Reconciliation, error) {
	if len(periods) == 0 {
		return nil, nil
	}

	selections := make([]Selection, len(periods))
	for i, p := range periods {
		// The line is irrelevant here -- the strip is over every row of the
		// period -- and NET SALES is named only because SelectionFor is the one
		// place that knows what a period's dates are once clipped to the range
		// actually asked for.
		sel, err := SelectionFor(req, p, SectionLine(NetSales))
		if err != nil {
			return nil, err
		}
		selections[i] = sel
	}

	balances := make([]int64, len(periods)+1)
	for i := range balances {
		var before time.Time
		if i < len(selections) {
			before = selections[i].From
		} else {
			before = selections[len(selections)-1].To.AddDate(0, 0, 1)
		}
		b, err := q.OpeningBalanceBefore(ctx, gendb.OpeningBalanceBeforeParams{
			EntityID:   pgUUID(req.EntityID),
			SourceKind: string(req.Basis),
			FromDate:   pgDate(before),
		})
		if err != nil {
			return nil, fmt.Errorf("report: balance before %s: %w", before.Format("2006-01-02"), err)
		}
		balances[i] = b
	}

	out := make([]Reconciliation, 0, len(periods))
	for i, p := range periods {
		sel := selections[i]
		r, err := q.ReconciliationForPeriod(ctx, gendb.ReconciliationForPeriodParams{
			EntityID:   pgUUID(sel.EntityID),
			SourceKind: string(sel.Basis),
			FromDate:   pgDate(sel.From),
			ToDate:     pgDate(sel.To),
		})
		if err != nil {
			return nil, fmt.Errorf("report: reconciling %s: %w", p, err)
		}

		opening, closing := balances[i], balances[i+1]
		out = append(out, Reconciliation{
			Period:           p,
			Opening:          money.Money{CurrencyCode: baseCcy, MinorUnits: opening},
			In:               money.Money{CurrencyCode: baseCcy, MinorUnits: r.InMinor},
			Out:              money.Money{CurrencyCode: baseCcy, MinorUnits: r.OutMinor},
			Transfers:        money.Money{CurrencyCode: baseCcy, MinorUnits: r.TransfersMinor},
			Closing:          money.Money{CurrencyCode: baseCcy, MinorUnits: closing},
			TransferRowCount: r.TransferRowCount,
			Derived:          true,
			Balances:         opening+r.InMinor-r.OutMinor-r.TransfersMinor == closing,
		})
	}
	return out, nil
}

func labelOf(code string) string {
	if l, ok := sectionLabels[code]; ok {
		return l
	}
	for _, l := range Chain {
		if l.Code == code {
			return l.Label
		}
	}
	return code
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
