package ingest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MyauDev/vekst/core/internal/ingest"
)

// fixtures are redacted copies of a real customer's Priorbank exports,
// produced by eval/anonymize.py. Names, tax identifiers and account numbers are
// replaced; every amount, balance, date and column layout is the bank's own.
//
// The two files are the two column layouts the same download produces: a
// rouble account carrying Корреспондент.УНП, and a currency account that drops
// it and adds the bank's own conversion instead. A parser that works on one and
// not the other passes half the real world.
func fixtures(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "priorbank-by", "*.csv"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures found: %v", err)
	}
	return paths
}

func parse(t *testing.T, path string) *ingest.Statement {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	st, err := ingest.ParsePriorbank(raw)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return st
}

// The strongest completeness check available, and the reason the fixtures keep
// their real amounts. If opening + credits - debits equals the declared closing
// balance, no row was lost, truncated or mis-signed.
func TestBalanceReconcilesOnEveryFixture(t *testing.T) {
	for _, path := range fixtures(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			st := parse(t, path)
			ok, diff, err := st.BalanceCheck()
			if err != nil {
				t.Fatalf("BalanceCheck: %v", err)
			}
			if !ok {
				t.Errorf("balance does not reconcile: opening %d, closing %d, difference %d %s over %d rows",
					st.Opening.MinorUnits, st.Closing.MinorUnits,
					diff.MinorUnits, diff.CurrencyCode, len(st.Rows))
			}
		})
	}
}

// The declared turnover is the bank's own sum. Comparing it against ours
// catches a row that parsed but landed in the wrong column, which the balance
// check alone would miss when the two errors cancel.
func TestDeclaredTurnoverMatchesParsedRows(t *testing.T) {
	for _, path := range fixtures(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			st := parse(t, path)
			var debits, credits int64
			for _, r := range st.Rows {
				debits += r.Debit.MinorUnits
				credits += r.Credit.MinorUnits
			}
			if debits != st.DeclaredDebit.MinorUnits {
				t.Errorf("debit turnover: parsed %d, bank declared %d", debits, st.DeclaredDebit.MinorUnits)
			}
			if credits != st.DeclaredCredit.MinorUnits {
				t.Errorf("credit turnover: parsed %d, bank declared %d", credits, st.DeclaredCredit.MinorUnits)
			}
		})
	}
}

// Both layouts must parse. The currency file has no УНП column at all, and a
// parser that reads columns by position rather than by name silently shifts
// every field after it.
func TestBothColumnLayoutsParse(t *testing.T) {
	var withTaxID, withoutTaxID int
	for _, path := range fixtures(t) {
		st := parse(t, path)
		if len(st.Rows) == 0 {
			t.Fatalf("%s: no rows", filepath.Base(path))
		}
		populated := 0
		for _, r := range st.Rows {
			if r.CounterpartyTaxID != "" {
				populated++
			}
		}
		if populated > 0 {
			withTaxID++
		} else {
			withoutTaxID++
		}
	}
	if withTaxID == 0 || withoutTaxID == 0 {
		t.Errorf("fixtures should cover both layouts: %d with a tax id column, %d without",
			withTaxID, withoutTaxID)
	}
}

// IMPLEMENTATION_PLAN.md §1.1 states the correctness check as "debit and credit
// are mutually exclusive -- fails when both columns are populated". Priorbank
// populates both on every row, one of them "0,00", so that check as written
// would reject every row of every file. It has to mean "both non-zero".
func TestDebitAndCreditAreBothPresentButNeverBothNonZero(t *testing.T) {
	for _, path := range fixtures(t) {
		st := parse(t, path)
		for _, r := range st.Rows {
			if r.Debit.MinorUnits != 0 && r.Credit.MinorUnits != 0 {
				t.Errorf("%s line %d: both debit and credit are non-zero", filepath.Base(path), r.LineNo)
			}
		}
	}
}

// An error has to name a line a human can find in Excel, so LineNo counts
// lines in the original file -- header block included -- not parsed rows.
func TestLineNumbersReferToTheOriginalFile(t *testing.T) {
	path := fixtures(t)[0]
	st := parse(t, path)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text, err := ingest.Decode(raw, ingest.DetectCharset(raw))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(text, "\n")

	first := st.Rows[0]
	if first.LineNo <= 1 {
		t.Fatalf("first row claims line %d; the header block alone is longer than that", first.LineNo)
	}
	if got := lines[first.LineNo-1]; !strings.HasPrefix(got, first.BookedOn) {
		t.Errorf("row says line %d, but that line is %q", first.LineNo, truncate(got, 40))
	}
}

func TestStatementMetadataIsRead(t *testing.T) {
	for _, path := range fixtures(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			st := parse(t, path)
			if st.Account == "" {
				t.Error("account not read from the preamble")
			}
			if st.Currency == "" {
				t.Error("currency not read")
			}
			if st.Opening.CurrencyCode != st.Currency || st.Closing.CurrencyCode != st.Currency {
				t.Errorf("balances are in %q/%q but the account is in %q",
					st.Opening.CurrencyCode, st.Closing.CurrencyCode, st.Currency)
			}
		})
	}
}

func TestNotAPriorbankExportIsRejected(t *testing.T) {
	_, err := ingest.ParsePriorbank([]byte("col a;col b\n1;2\n"))
	if err == nil {
		t.Fatal("a file with no Priorbank header parsed successfully")
	}
	if !strings.Contains(err.Error(), "header") {
		t.Errorf("error should say the header is missing, got %q", err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
