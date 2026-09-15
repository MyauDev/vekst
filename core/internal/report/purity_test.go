package report_test

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// The calculation is a pure function of its inputs, and this is what makes that
// a fact rather than an intention.
//
// A report reproduces six months later only if everything that shaped it was
// recorded beside it. A database handle, a clock or an environment variable
// reachable from `Compute` is an input nobody wrote down: the same rows and the
// same versions would produce a different table, and the difference would have
// no name. It is the same property the classification engine has, for the same
// reason.
//
// Enforced over imports rather than over behaviour because behaviour cannot be
// asserted absent. A test can show that today's Compute reads no clock; it
// cannot show that tomorrow's will not.
func TestTheCalculationImportsNothingImpure(t *testing.T) {
	const path = "pnl.go"

	// Allowed: formatting an error, sorting a slice, and the money type whose
	// whole purpose is to keep an amount out of a float.
	allowed := map[string]bool{
		"fmt":     true,
		"sort":    true,
		"strings": true,
		"github.com/MyauDev/vekst/core/internal/money": true,
	}

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("import path %s: %v", imp.Path.Value, err)
		}
		if allowed[p] {
			continue
		}
		t.Errorf("%s imports %q.\n"+
			"The calculation takes rows and returns a report: no database, no clock, no "+
			"environment. An input that is not among its arguments is one nothing recorded, "+
			"and a report that cannot be reproduced from what was recorded is not a report an "+
			"accountant can use. The reads belong in service.go; if this import is genuinely "+
			"pure, add it to the list above and say why.", path, p)
	}

	// The list is only load-bearing while it is short. A future entry that
	// looks harmless -- "time", say -- is exactly the one to argue about.
	for p := range allowed {
		if strings.HasPrefix(p, "database/") || p == "time" || p == "os" {
			t.Errorf("%q is on the allow list; it should not be", p)
		}
	}
}
