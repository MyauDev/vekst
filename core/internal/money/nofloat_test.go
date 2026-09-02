package money

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// moneyFieldName matches a struct field name that names a monetary amount:
// Amount, MinorUnits, Money, and variants like AmountMinor or BaseAmount.
// It deliberately does not match "MinorVersion" or similar false positives
// that merely contain a money-adjacent word without meaning one.
var moneyFieldName = regexp.MustCompile(`(?i)^(amount|minor|money).*$|.*(amount|money)$`)

// TestNoFloatMoneyFields walks every Go source file under /core -- generated
// and hand-written alike -- and fails if any struct field whose name marks
// it as money is declared as a floating-point type. ARCHITECTURE.md §6 asks
// for this explicitly, added before the first money column exists: that is
// the only moment it costs nothing to satisfy. See openspec design D5 and
// task 5.6 for the Python counterpart over the classifier's generated types.
func TestNoFloatMoneyFields(t *testing.T) {
	root := repoRoot(t)
	coreDir := filepath.Join(root, "core")

	fset := token.NewFileSet()
	var violations []string

	err := filepath.WalkDir(coreDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		ast.Inspect(file, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			for _, field := range st.Fields.List {
				ident, ok := field.Type.(*ast.Ident)
				if !ok || (ident.Name != "float32" && ident.Name != "float64") {
					continue
				}
				for _, name := range field.Names {
					if moneyFieldName.MatchString(name.Name) {
						pos := fset.Position(field.Pos())
						violations = append(violations, pos.String()+": field "+name.Name+" is "+ident.Name)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", coreDir, err)
	}

	for _, v := range violations {
		t.Error(v)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above " + wd)
		}
		dir = parent
	}
}
