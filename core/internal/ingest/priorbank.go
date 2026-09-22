package ingest

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/MyauDev/vekst/core/internal/money"
)

// Statement is one account's export: what the bank declared about the period,
// and the rows it contained.
type Statement struct {
	// Format is the parser that produced this, recorded on the batch so a
	// question about a report can be traced to the reader that answered it.
	Format string

	Account  string
	Currency string
	Holder   string

	// DeclaredRowCount is how many rows the file itself says it carries.
	// Priorbank declares none -- this stays nil for every statement this
	// parser produces -- but 1C ledger exports often do (ARCHITECTURE.md
	// §4a.1), and change 2.3's row-count check needs somewhere to read one
	// from once a parser exists that has one to give.
	DeclaredRowCount *int

	// PeriodFrom and PeriodTo are the bank's own declared statement period,
	// raw (ParseDate is applied by validation, the same as Row.BookedOn).
	// Empty when the preamble carries no period line -- change 2.3's
	// "declared period is covered continuously" check treats that as
	// nothing to check, not as a failure.
	PeriodFrom string
	PeriodTo   string

	// Declared by the bank, not computed by us. BalanceCheck compares the two,
	// and that comparison is the only proof available that no row was dropped.
	Opening        money.Money
	Closing        money.Money
	DeclaredDebit  money.Money
	DeclaredCredit money.Money

	// HasDeclaredBalance is false when the file carried no "Входящее сальдо"
	// / "Исходящее сальдо" summary rows at all -- 1C exports often omit them.
	// Opening and Closing stay at zero either way, which is indistinguishable
	// from a genuinely zero declared balance without this flag. Change 2.3's
	// tri-state balance_check_passed needs the distinction: NULL means "the
	// file declared no balances", not "they were zero".
	HasDeclaredBalance bool

	Rows []Row
}

// Row is one line of the statement, still in the bank's own terms. Turning it
// into a canonical transaction is change 2.5's job.
type Row struct {
	// LineNo is the 1-based line in the original file, kept so an error can
	// name a line a human can find. It survives every later stage.
	LineNo int

	BookedOn            string // raw; ParseDate is applied by validation
	DocumentNo          string
	CounterpartyName    string
	CounterpartyTaxID   string // УНП. Absent on currency accounts -- see below
	CounterpartyAccount string
	Description         string

	// RegulatedCode is a payment-purpose code assigned by someone other than
	// the payer: КНП in Kazakhstan, a 1C account number in a ledger export.
	// Priorbank carries none, so this is empty for every Belarusian row --
	// present anyway, because a code is stronger evidence than free text and
	// adding the column later means migrating transactions.
	RegulatedCode string

	// Debit and Credit are this package's own best-effort parse, kept for
	// Statement.BalanceCheck()'s convenience. They are not the authority on
	// whether the amount actually parses -- change 2.3's validation is, and
	// it re-parses DebitRaw/CreditRaw itself, with whatever locale an import
	// profile (2.4) may later override the detector's guess with. A row this
	// package could not parse carries a zero Money here, which is safe only
	// because such a row is always a correctness error (task 2.3), which
	// always rejects the batch regardless of what the balance difference
	// says -- see the comment on the row loop in ParsePriorbank.
	Debit  money.Money
	Credit money.Money

	// DebitRaw and CreditRaw are the bank's own text for the two columns,
	// unparsed. Validation is what actually decides whether an amount
	// parses (task 2.3); this package's own attempt above is provisional.
	DebitRaw  string
	CreditRaw string
}

func init() { Register(priorbankParser{}) }

// priorbankParser adapts ParsePriorbank to the Parser interface.
type priorbankParser struct{}

func (priorbankParser) Name() string { return "priorbank-by" }

func (priorbankParser) Parse(raw []byte, params *Parameters) (*Statement, error) {
	return ParsePriorbankWithParams(raw, params)
}

// Detect looks for the bank's own name in the preamble together with the
// header row. Either alone is too loose: another Belarusian bank could use the
// same Russian column names, and the word "Приорбанк" appears in the payment
// purpose of every bank-fee row in a statement from anywhere.
func (priorbankParser) Detect(raw []byte) bool {
	if len(raw) > 4096 {
		raw = raw[:4096]
	}
	text, err := Decode(raw, DetectCharset(raw))
	if err != nil {
		return false
	}
	return strings.Contains(text, "Приорбанк") && strings.Contains(text, "Дата док")
}

// ParsePriorbank reads a Priorbank (Belarus) CSV export.
//
// Three properties of this format decide the shape of this function, and all
// three were found in real exports rather than in documentation:
//
//   - **The column layout is not fixed per bank.** A rouble account carries
//     `Корреспондент.УНП`; a currency account drops it and adds
//     `Эквивалент.*`, the bank's own conversion to the base currency. Both
//     arrive in the same download. So columns are located by header name, and
//     an import profile that pins column positions would be wrong half the time.
//
//   - **The summary rows do not use the header's columns.** "Обороты" and
//     "Исходящее сальдо" put their figures in different positions entirely,
//     so they are read by taking the last two numeric cells rather than by
//     index. Reading them by header index silently mixes a currency account's
//     nominal movements with its rouble-equivalent balances.
//
//   - **Debit and credit are both always populated**, one of them "0,00".
//     `IMPLEMENTATION_PLAN.md` §1.1 states the correctness check as "debit and
//     credit are mutually exclusive -- fails when both columns populated",
//     which would reject every row of every file. The check has to be "both
//     non-zero".
//
//   - **The file carries `"` characters that are not CSV quoting.** The bank
//     wraps its own name in them on every export -- `"Приорбанк" Открытое
//     ...` -- and a counterparty's legal name often does too, since a quoted
//     name is ordinary Russian and Belarusian business writing, not a foreign
//     one. None of them ever protects a delimiter or a newline: every `;` in
//     this format is a real column boundary, quoted or not. `encoding/csv`
//     does not know that -- a field opening on `"` commits it to hunting for
//     a matching close, and one that never arrives (the bank's own name
//     closes on a space, not a delimiter) swallows the rest of the file into
//     a single field. Stripped before the reader ever sees them.
func ParsePriorbank(raw []byte) (*Statement, error) {
	return ParsePriorbankWithParams(raw, nil)
}

// ParsePriorbankWithParams is ParsePriorbank with add-import-profiles'
// overrides applied, field by field (design D1). A nil params, or any
// zero-valued field within one, reproduces ParsePriorbank's own behaviour
// exactly (task 6.1) -- Parameters.sourceColumn and the checks below all
// fall back to this parser's built-in choice whenever a field says nothing.
func ParsePriorbankWithParams(raw []byte, params *Parameters) (*Statement, error) {
	charset := DetectCharset(raw)
	if params != nil && params.Charset != "" {
		charset = params.Charset
	}
	text, err := Decode(raw, charset)
	if err != nil {
		return nil, err
	}
	// See the fourth bullet above: a literal `"` here is always prose, never
	// CSV quoting, and encoding/csv cannot tell the two apart.
	text = strings.ReplaceAll(text, `"`, "")

	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = ';'
	if params != nil && params.Delimiter != 0 {
		reader.Comma = rune(params.Delimiter)
	}
	reader.FieldsPerRecord = -1 // ragged on purpose: preamble and totals differ
	reader.LazyQuotes = true

	// Read record by record rather than with ReadAll, because the line number
	// has to be the line number *in the file*. encoding/csv silently skips
	// blank lines, and this format has one between the preamble and the header,
	// so a record index is one short of the truth from there on. FieldPos gives
	// the real line, which is what an error report has to quote: somebody will
	// go looking for it in Excel.
	records, lines, err := readAllWithLines(reader)
	if err != nil {
		return nil, fmt.Errorf("ingest: priorbank: reading csv: %w", err)
	}

	headerAt := -1
	for i, rec := range records {
		if len(rec) > 0 && strings.HasPrefix(strings.TrimSpace(rec[0]), "Дата док") {
			headerAt = i
			break
		}
	}
	if headerAt < 0 {
		return nil, fmt.Errorf("ingest: priorbank: no header row; not a Priorbank export")
	}

	col := map[string]int{}
	for i, name := range records[headerAt] {
		col[strings.TrimSpace(name)] = i
	}
	debitAt, okD := col[params.sourceColumn("debit", "Номинал.Дебет")]
	creditAt, okC := col[params.sourceColumn("credit", "Номинал.Кредит")]
	if !okD || !okC {
		return nil, fmt.Errorf("ingest: priorbank: header has no nominal debit/credit columns")
	}
	// Present only on currency accounts. Its presence also tells the summary
	// reader which pair of numbers is the nominal one.
	_, hasEquivalent := col["Эквивалент.Дебет"]

	st := &Statement{}
	parsePreamble(records[:headerAt], st)
	if st.Currency == "" {
		st.Currency = "BYN"
	}

	zero, err := money.New(st.Currency, 0)
	if err != nil {
		return nil, fmt.Errorf("ingest: priorbank: account currency: %w", err)
	}
	st.Opening, st.Closing = zero, zero
	st.DeclaredDebit, st.DeclaredCredit = zero, zero

	for i, rec := range records {
		lineNo := lines[i]
		first := strings.TrimSpace(cell(rec, 0))

		switch {
		case strings.HasPrefix(first, "Входящее сальдо"):
			if st.Opening, err = summaryBalance(rec, st.Currency, hasEquivalent); err != nil {
				return nil, fmt.Errorf("ingest: priorbank: line %d: opening balance: %w", lineNo, err)
			}
			if cur := currencyFromSummary(rec); cur != "" {
				st.Currency = cur
			}
			st.HasDeclaredBalance = true
		case strings.HasPrefix(first, "Исходящее сальдо"):
			if st.Closing, err = summaryBalance(rec, st.Currency, hasEquivalent); err != nil {
				return nil, fmt.Errorf("ingest: priorbank: line %d: closing balance: %w", lineNo, err)
			}
			st.HasDeclaredBalance = true
		case strings.HasPrefix(first, "Обороты"):
			d, c, err := summaryPair(rec, st.Currency, hasEquivalent)
			if err != nil {
				return nil, fmt.Errorf("ingest: priorbank: line %d: turnover: %w", lineNo, err)
			}
			st.DeclaredDebit, st.DeclaredCredit = d, c
		case datePrefix.MatchString(first):
			row, err := parseRow(rec, lineNo, col, debitAt, creditAt, st.Currency, params)
			if err != nil {
				return nil, fmt.Errorf("ingest: priorbank: line %d: %w", lineNo, err)
			}
			st.Rows = append(st.Rows, row)
		}
	}

	return st, nil
}

var (
	datePrefix   = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}$`)
	numericCell  = regexp.MustCompile(`^[\d \x{00a0}]+,\d{2}$`)
	currencyCell = regexp.MustCompile(`^[A-Z]{3}$`)
	accountLine  = regexp.MustCompile(`^Счет клиента\**\s*(\S+)(?:\s+([A-Z]{3}))?`)
	holderLine   = regexp.MustCompile(`^Наименование счета\**\s*(.+)$`)
	periodLine   = regexp.MustCompile(`^ВЫПИСКА ПО СЧЕТУ \S+ С (\d{2}\.\d{2}\.\d{4}) ПО (\d{2}\.\d{2}\.\d{4})`)
)

// readAllWithLines returns every record together with the line it started on.
func readAllWithLines(r *csv.Reader) ([][]string, []int, error) {
	var records [][]string
	var lines []int
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			return records, lines, nil
		}
		if err != nil {
			return nil, nil, err
		}
		line, _ := r.FieldPos(0)
		records = append(records, rec)
		lines = append(lines, line)
	}
}

func parsePreamble(records [][]string, st *Statement) {
	for _, rec := range records {
		line := strings.TrimSpace(cell(rec, 0))
		if m := accountLine.FindStringSubmatch(line); m != nil {
			st.Account = m[1]
			if m[2] != "" {
				st.Currency = m[2]
			}
			continue
		}
		if m := holderLine.FindStringSubmatch(line); m != nil {
			st.Holder = strings.TrimSpace(m[1])
			continue
		}
		if m := periodLine.FindStringSubmatch(line); m != nil {
			st.PeriodFrom, st.PeriodTo = m[1], m[2]
		}
	}
}

// summaryBalance reads a balance row, which is signed as credit minus debit:
// the account is passive, so a credit balance is what the customer holds.
func summaryBalance(rec []string, currency string, hasEquivalent bool) (money.Money, error) {
	debit, credit, err := summaryPair(rec, currency, hasEquivalent)
	if err != nil {
		return money.Money{}, err
	}
	return money.New(currency, credit.MinorUnits-debit.MinorUnits)
}

// summaryPair pulls the nominal debit and credit out of a summary row.
//
// These rows ignore the header's column positions, so the numbers are found by
// scanning for numeric cells. On a currency account the nominal pair comes
// first and the base-currency equivalent second; on a rouble account there is
// only one pair. Taking the wrong pair compares EUR movements against BYN
// balances, which looks like a 153,000 discrepancy and is really a unit error.
func summaryPair(rec []string, currency string, hasEquivalent bool) (money.Money, money.Money, error) {
	var found []string
	for _, c := range rec {
		if numericCell.MatchString(strings.TrimSpace(c)) {
			found = append(found, c)
		}
	}
	if len(found) < 2 {
		return money.Money{}, money.Money{}, fmt.Errorf("expected two amounts, found %d", len(found))
	}
	pair := found[len(found)-2:]
	if hasEquivalent {
		pair = found[:2]
	}
	debit, err := ParseAmount(currency, pair[0])
	if err != nil {
		return money.Money{}, money.Money{}, err
	}
	credit, err := ParseAmount(currency, pair[1])
	if err != nil {
		return money.Money{}, money.Money{}, err
	}
	return debit, credit, nil
}

func currencyFromSummary(rec []string) string {
	for _, c := range rec {
		if currencyCell.MatchString(strings.TrimSpace(c)) {
			return strings.TrimSpace(c)
		}
	}
	return ""
}

// parseRow never fails on a malformed amount. A single bad row is the
// customer's problem, reported by change 2.3 with the line this row still
// carries -- it is not a defect that should take the rest of their file
// down with it, which returning an error here (and aborting ParsePriorbank
// entirely) used to do. debit and credit fall back to a zero Money in their
// row's currency when they fail to parse; DebitRaw and CreditRaw always
// carry the original text regardless, which is what validation actually
// checks.
//
// BookedOn is deliberately not one of column_map's overridable fields for
// this parser: reading column 0 is also how a data row is told apart from a
// summary row (datePrefix.MatchString(first) in ParsePriorbankWithParams),
// so moving where the date is read from would move the row boundary itself,
// not just a canonical field mapping. document_ref, counterparty and
// description are ordinary header lookups and take an override freely.
func parseRow(rec []string, lineNo int, col map[string]int, debitAt, creditAt int, currency string, params *Parameters) (Row, error) {
	debitRaw, creditRaw := cell(rec, debitAt), cell(rec, creditAt)

	debit, err := ParseAmount(currency, debitRaw)
	if err != nil {
		if debit, err = money.New(currency, 0); err != nil {
			return Row{}, fmt.Errorf("row currency: %w", err)
		}
	}
	credit, err := ParseAmount(currency, creditRaw)
	if err != nil {
		if credit, err = money.New(currency, 0); err != nil {
			return Row{}, fmt.Errorf("row currency: %w", err)
		}
	}
	return Row{
		LineNo:              lineNo,
		BookedOn:            strings.TrimSpace(cell(rec, 0)),
		DocumentNo:          strings.TrimSpace(named(rec, col, params.sourceColumn("document_ref", "N док."))),
		CounterpartyName:    strings.TrimSpace(named(rec, col, params.sourceColumn("counterparty", "Корреспондент.Название"))),
		CounterpartyTaxID:   strings.TrimSpace(named(rec, col, "Корреспондент.УНП")),
		CounterpartyAccount: strings.TrimSpace(named(rec, col, "Корреспондент.Счет")),
		Description:         strings.TrimSpace(named(rec, col, params.sourceColumn("description", "Назначение"))),
		Debit:               debit,
		Credit:              credit,
		DebitRaw:            strings.TrimSpace(debitRaw),
		CreditRaw:           strings.TrimSpace(creditRaw),
	}, nil
}

func named(rec []string, col map[string]int, name string) string {
	i, ok := col[name]
	if !ok {
		return ""
	}
	return cell(rec, i)
}

func cell(rec []string, i int) string {
	if i < 0 || i >= len(rec) {
		return ""
	}
	return rec[i]
}

// BalanceCheck reports whether opening + credits - debits equals the declared
// closing balance, using the rows actually parsed.
//
// This is the strongest completeness check available and it has zero tolerance:
// if it holds, no row was lost, truncated or mis-signed. Verified against every
// fixture in core/testdata, both column layouts.
//
// It returns the difference so a caller can show it. Change 2.3 decides what a
// non-zero difference means for the batch; this package only measures.
func (s *Statement) BalanceCheck() (ok bool, difference money.Money, err error) {
	var debits, credits int64
	for _, r := range s.Rows {
		debits += r.Debit.MinorUnits
		credits += r.Credit.MinorUnits
	}
	expected := s.Opening.MinorUnits + credits - debits
	difference, err = money.New(s.Currency, expected-s.Closing.MinorUnits)
	if err != nil {
		return false, money.Money{}, err
	}
	return difference.MinorUnits == 0, difference, nil
}
