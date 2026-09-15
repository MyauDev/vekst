package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Task 5.6, the half of the round trip that lives here: the line number
// survives the persist boundary.
//
// It was lost at exactly one place. `Row.LineNo` is the line in the *original*
// file -- `TestLineNumbersReferToTheOriginalFile` pins that it is not the index
// of a parsed row, which differs the moment a file has a preamble, and every
// real bank export does -- and `buildTransaction` dropped it while copying
// every other field across. Nothing noticed, because nothing downstream asked
// for it until a customer wanted to check a figure against their own file.
//
// Asserted against a committed fixture rather than a constructed row, because
// what makes the number worth anything is that it indexes a real file: the test
// reads the line it claims and checks the date on that line is the date on the
// transaction.
func TestTheLineNumberSurvivesIntoTheTransaction(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "priorbank-by", "*.csv"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures found: %v", err)
	}
	path := paths[0]

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	st, err := Parse(raw)
	if err != nil {
		t.Fatalf("parsing %s: %v", filepath.Base(path), err)
	}
	if len(st.Rows) == 0 {
		t.Fatalf("%s parsed to no rows", filepath.Base(path))
	}

	text, err := Decode(raw, DetectCharset(raw))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(text, "\n")

	entity, account, batch := uuid.New(), uuid.New(), uuid.New()
	for _, r := range st.Rows {
		txn, err := buildTransaction(entity, account, batch, "bank", st.Currency, r)
		if err != nil {
			t.Fatalf("line %d: buildTransaction: %v", r.LineNo, err)
		}

		if txn.LineNo != int32(r.LineNo) {
			t.Fatalf("parsed row says line %d, the transaction says %d.\n"+
				"The batch says which file and the line says where in it; either alone is "+
				"half a provenance, and a customer checking this figure has the file open.",
				r.LineNo, txn.LineNo)
		}
		if txn.LineNo <= 0 {
			t.Fatalf("line_no is %d: the database refuses it, and rightly -- 0 is the "+
				"sentinel that would stand in for a line in somebody's file", txn.LineNo)
		}
		if int(txn.LineNo) > len(lines) {
			t.Fatalf("line %d is past the end of a %d-line file", txn.LineNo, len(lines))
		}

		// The number indexes the file it claims to index.
		if got := lines[txn.LineNo-1]; !strings.HasPrefix(got, r.BookedOn) {
			t.Errorf("transaction claims line %d, but that line of %s is %q",
				txn.LineNo, filepath.Base(path), truncateLine(got, 40))
		}
	}
}

func truncateLine(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
