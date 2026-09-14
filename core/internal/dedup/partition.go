package dedup

import (
	"github.com/MyauDev/vekst/core/internal/ledger"
)

// Candidate is one row a persist job is about to write, before its
// DedupHash is known.
type Candidate struct {
	LineNo int
	Txn    ledger.Transaction
}

// Partition assigns every candidate its DedupHash (design D5, occurrence
// included, via ledger.DedupHash).
//
// This is D2's whole implementation, and design D2 turned out to need no
// skip logic at all: occurrence is exactly the Nth time
// ledger.ContentKey(candidate) repeats within this slice, a plain
// incrementing counter, so it is a bijection between (content, its Kth
// repeat) and a occurrence number -- two candidates can share a DedupHash
// here only if a caller hands Partition the same content-key at the same
// repeat count twice in one call, which cannot happen from a single pass
// over one slice counting as it goes. Two genuinely distinct payments
// (design D5's own two coffees) get occurrences 1 and 2 and are both kept,
// correctly, by construction rather than by a check.
//
// An earlier version of this function kept a second map to skip a
// "repeated hash" as level D2 -- confirmed, while adding this function's
// own tests, to be dead code: nothing can reach it without a SHA-256
// collision. Removed rather than kept as a defensive-looking no-op
// (CLAUDE.md: no validation for a scenario that cannot happen). What
// actually stands in for "this job processed one line twice" is
// persistWorker's own transaction: a failed attempt rolls back everything
// it wrote, so a retry recomputes occurrence fresh and never sees a
// partial result from the attempt before it.
func Partition(candidates []Candidate) []Candidate {
	seen := map[string]int{}
	keep := make([]Candidate, len(candidates))
	for i, c := range candidates {
		key := ledger.ContentKey(c.Txn)
		seen[key]++
		c.Txn.DedupHash = ledger.DedupHash(c.Txn, seen[key])
		keep[i] = c
	}
	return keep
}
