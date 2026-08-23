package server_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Task 5.3. The health path must open no database connection and touch no
// tenant-scoped table. Today that is trivially true because there is no
// database; the point is that it stays true after change 0.2 adds one.
//
// This is a structural test rather than a behavioural one because the failure
// it guards against is a future edit, not a current bug. A behavioural test
// would pass forever by accident.
func TestHealthPathImportsNoDatabase(t *testing.T) {
	forbidden := []string{
		"github.com/jackc/pgx",
		"database/sql",
		"github.com/riverqueue/river",
		"github.com/MyauDev/vekst/core/internal/db",
	}

	fset := token.NewFileSet()
	pkg, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse server package: %v", err)
	}

	for name, p := range pkg {
		for file, f := range p.Files {
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				for _, bad := range forbidden {
					if strings.HasPrefix(path, bad) {
						t.Errorf(
							"%s (package %s) imports %q.\n"+
								"The health path must not reach a database: liveness answers "+
								"\"is this process broken\", not \"is the system healthy\". "+
								"A liveness probe that checks a dependency turns a recoverable "+
								"outage into a crash loop. Change 0.2 adds /readyz for the "+
								"dependency check; put it there.",
							filepath.Base(file), name, path,
						)
					}
				}
			}
		}
	}
}

// The classifier package is the other half of the same rule: ARCHITECTURE.md
// A-4 says the classifier never receives database credentials. core's client
// adapter must not hand it any either.
func TestClassifyPackageImportsNoDatabase(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "../../classify", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse classify package: %v", err)
	}

	for _, p := range pkgs {
		for file, f := range p.Files {
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if strings.HasPrefix(path, "github.com/jackc/pgx") || path == "database/sql" {
					t.Errorf("%s imports %q; see ARCHITECTURE.md A-4", filepath.Base(file), path)
				}
			}
		}
	}
}

var _ = ast.Print
