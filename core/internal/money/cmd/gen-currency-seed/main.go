// gen-currency-seed writes migration 00018's currencies seed rows from
// core/internal/money's exponent map -- the codebase's existing canonical
// currency data. The map is Go data, not eval/'s domain (eval/emit.py reads a
// taxonomy source outside the repository and has no reason to know about
// money), so this is a separate, small generator rather than a reuse of that
// one. See design.md §2.2 for change connect-app-end-to-end.
//
// Run: go run ./core/internal/money/cmd/gen-currency-seed
//
// Output is the exact row text migration 00018 embeds, one row per line,
// terminated with a comma except the last, which scripts/check-currency-seed.sh
// diffs against the migration's own copy -- so this program's output and the
// migration's INSERT block must always read identically.
package main

import (
	"fmt"

	"github.com/MyauDev/vekst/core/internal/money"
)

func main() {
	codes := money.Codes()
	for i, code := range codes {
		exp, ok := money.Exponent(code)
		if !ok {
			panic("money: Codes() returned a code Exponent() does not know: " + code)
		}
		sep := ","
		if i == len(codes)-1 {
			sep = ";"
		}
		fmt.Printf("  (%s, %d)%s\n", sqlString(code), exp, sep)
	}
}

// sqlString renders s as a single-quoted SQL string literal. Currency codes
// never contain a quote, but escaping unconditionally costs nothing and keeps
// this generator honest if that ever stops being true.
func sqlString(s string) string {
	escaped := ""
	for _, r := range s {
		if r == '\'' {
			escaped += "''"
			continue
		}
		escaped += string(r)
	}
	return "'" + escaped + "'"
}
