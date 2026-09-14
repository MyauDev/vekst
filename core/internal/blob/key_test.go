package blob

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The key is built from identifiers only. A user-supplied file name is a path
// traversal and a collision at once (design task 4.4); this asserts the
// generator never has the chance to read one.
func TestKeyIsBuiltFromIdentifiersOnly(t *testing.T) {
	org := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	batch := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	got := Key(org, batch)
	want := "org/11111111-1111-1111-1111-111111111111/batch/22222222-2222-2222-2222-222222222222"
	if got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
}

func TestKeyDoesNotCollideAcrossOrganisations(t *testing.T) {
	batch := uuid.New()
	a := Key(uuid.New(), batch)
	b := Key(uuid.New(), batch)
	if a == b {
		t.Error("two organisations produced the same key for the same batch id")
	}
	if !strings.Contains(a, batch.String()) || !strings.Contains(b, batch.String()) {
		t.Error("key does not carry the batch id it was built from")
	}
}
