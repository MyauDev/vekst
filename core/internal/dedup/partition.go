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

// Partition is D2: it assigns every candidate its DedupHash (design D5,
// occurrence included, via ledger.DedupHash) and separates out any
// candidate whose (content, occurrence) pair already appeared earlier in
// this same call.
//
// That case does not happen for rows processed once each in one pass --
// occurrence is exactly the Nth time ledger.ContentKey repeats within this
// slice, so two genuinely distinct payments (design D5's own two coffees)
// get occurrences 1 and 2 and never collide here. This exists as a defence
// against this job somehow being handed the same line twice -- a raw_rows
// read that ran twice, a retry that did not deduplicate its own input --
// which is a defect in the caller, not a fact about the customer's file.
// See ledger.DedupHash's own doc comment, and the confirmation this
// package's own design note records: two genuinely repeated rows within
// one file are correct data and are both kept, never skipped as D2.
func Partition(candidates []Candidate) (keep []Candidate, skips []Skip) {
	seen := map[string]int{}
	hashSeen := map[string]bool{}

	for _, c := range candidates {
		key := ledger.ContentKey(c.Txn)
		seen[key]++
		occurrence := seen[key]
		hash := ledger.DedupHash(c.Txn, occurrence)

		if hashSeen[hash] {
			skips = append(skips, Skip{
				LineNo:    c.LineNo,
				PostingNo: c.Txn.PostingNo,
				Level:     LevelD2,
				DedupHash: hash,
			})
			continue
		}
		hashSeen[hash] = true

		c.Txn.DedupHash = hash
		keep = append(keep, c)
	}
	return keep, skips
}
