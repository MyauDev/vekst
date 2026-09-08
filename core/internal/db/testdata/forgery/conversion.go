// This file must NOT compile. See composite_literal.go.
//
// The forgery: converting a raw UUID to an OrgID. This is what `type OrgID
// uuid.UUID` would have permitted from any package, and the reason the type is
// a struct with an unexported field instead.
package main

import (
	"github.com/google/uuid"

	"github.com/MyauDev/vekst/core/internal/db"
)

func main() {
	org := db.OrgID(uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))
	_ = org
}
