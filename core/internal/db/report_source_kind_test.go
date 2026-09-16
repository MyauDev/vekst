package db

import (
	"os"
	"strings"
	"testing"
)

// A report line is computed from one `source_kind`, and this is the test that
// keeps it true as the file grows.
//
// ARCHITECTURE.md 5.1 calls mixing ledger and bank rows the single most likely
// way this product prints a wrong number: an invoice and the payment that
// settles it are two rows describing one event, and a line summing both counts
// the money twice. Nothing about the result looks wrong -- the total is
// plausible, the currency is right, every row genuinely exists.
//
// The failure mode is a query added later by somebody solving a different
// problem, who joins a table and filters on a date and an entity because those
// are the filters the neighbouring query has. A join that was never written
// cannot be asserted on, so the assertion is made over the file instead: every
// statement in report.sql names source_kind, or this fails with the name of the
// one that does not.
func TestNoReportQueryOmitsSourceKind(t *testing.T) {
	const path = "query/report.sql"

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	statements := strings.Split(string(raw), "-- name: ")
	if len(statements) < 2 {
		t.Fatalf("%s contains no sqlc statements; has the file moved?", path)
	}

	for _, s := range statements[1:] {
		name, body, ok := strings.Cut(s, "\n")
		if !ok {
			t.Fatalf("statement %q has no body", name)
		}
		name = strings.TrimSpace(name)

		// Comment lines carry the reasoning, and the word appears in several
		// of them. Only the SQL counts.
		var sql strings.Builder
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue
			}
			sql.WriteString(line)
			sql.WriteByte('\n')
		}

		if !strings.Contains(sql.String(), "source_kind") {
			t.Errorf("%s does not filter on source_kind.\n"+
				"A report line is computed from one source kind and never from both: "+
				"summing ledger and bank rows together counts an invoice and its payment twice "+
				"(ARCHITECTURE.md 5.1). If this query genuinely reads across both -- a "+
				"reconciliation count, say -- it does not belong in this file.", name)
		}
	}
}
