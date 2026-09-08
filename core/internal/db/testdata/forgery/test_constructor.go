// This file must NOT compile. See composite_literal.go.
//
// The forgery: reaching the test-only constructor without supplying a
// testing.TB. The parameter is what forces a caller to import "testing", which
// is greppable in CI and obvious in review; without it this would be a fourth
// door taking a raw UUID.
package main

import (
	"github.com/google/uuid"

	"github.com/MyauDev/vekst/core/internal/db"
)

func main() {
	org := db.OrgIDForTest(uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))
	_ = org
}
