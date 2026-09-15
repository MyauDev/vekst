package dedup

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MyauDev/vekst/core/internal/ledger"
	"github.com/MyauDev/vekst/core/internal/money"
)

func testCandidate(lineNo int, description string) Candidate {
	return Candidate{
		LineNo: lineNo,
		Txn: ledger.Transaction{
			AccountID:       uuid.New(),
			BookedOn:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Amount:          money.Money{CurrencyCode: "NOK", MinorUnits: 500},
			DescriptionNorm: description,
		},
	}
}

// Two genuinely distinct candidates -- different content -- both come back
// with distinct hashes.
func TestPartitionAssignsDistinctHashesToDistinctCandidates(t *testing.T) {
	got := Partition([]Candidate{
		testCandidate(1, "coffee"),
		testCandidate(2, "rent"),
	})
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].Txn.DedupHash == "" || got[1].Txn.DedupHash == "" {
		t.Error("Partition did not assign a DedupHash to a candidate")
	}
	if got[0].Txn.DedupHash == got[1].Txn.DedupHash {
		t.Error("two distinct candidates were assigned the same hash")
	}
}

// Two candidates with identical content -- design D5's own two coffees --
// both come back, at occurrences 1 and 2, with different hashes: this is
// exactly what makes them both storable rather than one mistaken for a
// duplicate of the other.
func TestPartitionGivesRepeatedContentDifferentOccurrences(t *testing.T) {
	got := Partition([]Candidate{
		testCandidate(1, "coffee"),
		testCandidate(2, "coffee"),
	})
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].Txn.DedupHash == got[1].Txn.DedupHash {
		t.Error("occurrence 1 and occurrence 2 of the same content were assigned the same hash")
	}
}

// Even the exact same Candidate value, handed to Partition twice, is not a
// hash collision: occurrence is a plain per-call counter over content
// repeats, so the second copy simply becomes occurrence 2 of that content
// -- confirmed here because an earlier version of this function assumed
// the opposite (see Partition's own doc comment for what that assumption
// cost).
func TestPartitionOfTheSameCandidateTwiceStillGivesTwoHashes(t *testing.T) {
	c := testCandidate(1, "coffee")
	got := Partition([]Candidate{c, c})
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].Txn.DedupHash == got[1].Txn.DedupHash {
		t.Error("two occurrences of the same candidate were assigned the same hash")
	}
}
