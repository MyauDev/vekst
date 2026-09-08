// This file must NOT compile. It is compiled deliberately by
// TestOrgIDCannotBeForgedOutsideThisPackage, which fails if it succeeds.
//
// The forgery: building an OrgID by naming its field. The field is unexported,
// so no package outside db can reach it.
package main

import (
	"github.com/google/uuid"

	"github.com/MyauDev/vekst/core/internal/db"
)

func main() {
	// The organisation an attacker named, straight off the wire.
	org := db.OrgID{v: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")}
	_ = org
}
