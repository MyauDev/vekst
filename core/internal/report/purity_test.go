package report_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The calculation and the cell addressing are pure functions of their inputs,
// and this is what makes that a fact rather than an intention.
//
// A report reproduces six months later only if everything that shaped it was
// recorded beside it. A database handle, a clock or an environment variable
// reachable from `Compute` or `SelectionFor` is an input nobody wrote down: the
// same rows and the same versions would produce a different table, and the
// difference would have no name. It is the same property the classification
// engine has, for the same reason.
//
// Enforced over the source rather than over behaviour, because behaviour cannot
// be asserted absent. A test can show that today's Compute reads no clock; it
// cannot show that tomorrow's will not.
var pureFiles = []string{"pnl.go", "selection.go"}

func TestTheCalculationImportsNothingImpure(t *testing.T) {
	// Allowed: formatting an error, sorting a slice, string handling, the
	// money type whose whole purpose is to keep an amount out of a float, the
	// identifier type a cell is addressed by, and calendar arithmetic.
	//
	// `time` is on this list and it is the entry to argue about. What is
	// forbidden is a *clock* -- a value that differs between two runs with the
	// same inputs -- not date arithmetic, and the second half of this test is
	// what keeps the distinction honest.
	allowed := map[string]bool{
		"fmt":                    true,
		"sort":                   true,
		"strings":                true,
		"time":                   true,
		"github.com/google/uuid": true,
		"github.com/MyauDev/vekst/core/internal/money": true,
	}

	for _, path := range pureFiles {
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
				"These files take rows and arguments and return a report: no database, no "+
				"clock, no environment. An input that is not among their arguments is one "+
				"nothing recorded, and a report that cannot be reproduced from what was "+
				"recorded is not a report an accountant can use. The reads belong in "+
				"service.go; if this import is genuinely pure, add it to the list above and "+
				"say why.", path, p)
		}
	}
}

// The calls that would make a second run of the same inputs produce a different
// answer. Import lists cannot catch these on their own: `time` is needed for
// calendar arithmetic and forbidden as a clock, and the difference is the
// selector, not the package.
func TestTheCalculationReadsNoClockAndNoEnvironment(t *testing.T) {
	forbidden := map[string]string{
		"time.Now":       "a clock: two runs of one report would differ, and the difference would have no name",
		"time.Since":     "a clock",
		"os.Getenv":      "configuration that travels with the process rather than with the report",
		"os.LookupEnv":   "configuration that travels with the process rather than with the report",
		"rand.Int":       "an answer that is not a function of its inputs",
		"rand.Intn":      "an answer that is not a function of its inputs",
		"uuid.New":       "identity minted at read time, which is state the report does not have",
		"uuid.NewString": "identity minted at read time, which is state the report does not have",
	}

	for _, path := range pureFiles {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			name := pkg.Name + "." + sel.Sel.Name
			if why, bad := forbidden[name]; bad {
				t.Errorf("%s:%d calls %s -- %s",
					path, fset.Position(sel.Pos()).Line, name, why)
			}
			return true
		})
	}
}

// The list above is only load-bearing while somebody can see it. A file added
// to the package that belongs among the pure ones and is not listed here is
// unguarded, and the guard's absence is invisible -- so the test says which
// files it did not check.
func TestEveryFileIsEitherGuardedOrAService(t *testing.T) {
	// service.go holds the reads; the _test.go files are tests.
	known := map[string]bool{"service.go": true, "drilldown.go": true}
	for _, f := range pureFiles {
		known[f] = true
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !known[name] {
			t.Errorf("%s is neither guarded as pure nor known to hold the reads.\n"+
				"Add it to pureFiles if the calculation lives in it, or to the known list "+
				"above if it opens a database -- but do not leave it unclassified, because "+
				"an unguarded file is exactly where a clock gets in.", name)
		}
	}
}
