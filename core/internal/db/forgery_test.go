package db

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Task 3.5. The guarantee behind design D7 is a compile-time one, so the test
// for it has to be a compilation.
//
// Each file under testdata/forgery is a way someone might try to build an
// OrgID from a value they were handed rather than one they established, and
// each must fail to compile. A comment saying "don't do this" is not the same
// claim: it survives a refactor that exports the field, and this does not.
//
// testdata is ignored by the go tool when it matches ./..., so these files are
// invisible to a normal build and are compiled only here, by explicit path.
func TestOrgIDCannotBeForgedOutsideThisPackage(t *testing.T) {
	for _, tc := range []struct {
		file string
		// A fragment of the compiler's complaint. Asserted so that the test
		// fails loudly if the file stops compiling for an unrelated reason --
		// a typo, a renamed import -- which would otherwise look like proof.
		wantError string
	}{
		{
			file:      "composite_literal.go",
			wantError: "cannot refer to unexported field v",
		},
		{
			file:      "conversion.go",
			wantError: "cannot convert",
		},
		{
			file:      "test_constructor.go",
			wantError: "want (testing.TB, uuid.UUID)",
		},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join("testdata", "forgery", tc.file)
			out, err := exec.Command("go", "build", "-o", "/dev/null", path).CombinedOutput()
			if err == nil {
				t.Fatalf("%s compiled; an OrgID can be built outside package db, so "+
					"row-level security would isolate a transaction to whatever "+
					"organisation the caller named", tc.file)
			}
			if !strings.Contains(string(out), tc.wantError) {
				t.Fatalf("%s failed to compile, but not for the expected reason.\n"+
					"want a message containing: %s\ngot:\n%s", tc.file, tc.wantError, out)
			}
		})
	}
}

// The counterpart, and the reason the test above is not merely asserting that
// nothing compiles: the sanctioned constructors do work, from inside the
// package and -- for OrgIDForTest -- from any test that imports it.
func TestTheSanctionedConstructorsWork(t *testing.T) {
	if org := OrgIDForNewOrg(); org.IsZero() {
		t.Error("OrgIDForNewOrg returned the zero value")
	}
	if org := OrgIDForTest(t, OrgIDForNewOrg().UUID()); org.IsZero() {
		t.Error("OrgIDForTest returned the zero value")
	}
}
