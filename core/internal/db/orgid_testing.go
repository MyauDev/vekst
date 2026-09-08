package db

import (
	"testing"

	"github.com/google/uuid"
)

// OrgIDForTest builds an OrgID from a raw identifier, for tests that need to
// act for an organisation without a session, a job or a creation.
//
// It takes a testing.TB it does not use. That parameter is the entire point:
// production code cannot call this without importing "testing", which is
// obvious in review, greppable in CI, and a dependency no serving binary has
// any business carrying. Without it, this would be a fourth door -- one that
// takes a raw UUID and asks no questions -- reachable from anywhere in the
// codebase.
//
// This file is deliberately not _test.go: tests in *other* packages need it,
// and a _test.go file is not importable.
func OrgIDForTest(tb testing.TB, id uuid.UUID) OrgID {
	tb.Helper()
	if id == uuid.Nil {
		tb.Fatal("OrgIDForTest called with the nil UUID; InTx would reject it")
	}
	return OrgID{v: id}
}
