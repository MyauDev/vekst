package normalize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Task 4.3. The Go implementation and eval/norm.py must agree on every input
// in eval/out/normalize_conformance.json, which eval/conformance.py produces
// from the redacted Priorbank fixtures plus the cases those fixtures happen
// not to contain.
//
// The fixture is committed rather than generated here on purpose. CI has no
// access to ../docCl and no reason to run Python to test Go; what it needs is
// the reference's answers, and those are a file. Regenerating it is a
// deliberate act with a diff to review -- which is exactly the ceremony a
// change to what counts as a match should carry.
type conformance struct {
	NormalizeVersion string `json:"normalize_version"`
	Descriptions     []struct {
		In  string `json:"in"`
		Out string `json:"out"`
	} `json:"descriptions"`
	Keys []struct {
		Name    string `json:"name"`
		TaxID   string `json:"tax_id"`
		Account string `json:"account"`
		Key     string `json:"key"`
		Tier    string `json:"tier"`
	} `json:"keys"`
}

func load(t *testing.T) conformance {
	t.Helper()
	path := filepath.Join("..", "..", "..", "eval", "out", "normalize_conformance.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the reference fixture: %v", err)
	}
	var c conformance
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	// An empty fixture would make every assertion below pass without
	// comparing anything, which is the one failure mode a conformance test
	// cannot afford.
	if len(c.Descriptions) == 0 || len(c.Keys) == 0 {
		t.Fatalf("fixture is empty: %d descriptions, %d keys", len(c.Descriptions), len(c.Keys))
	}
	return c
}

func TestFixtureIsForThisVersion(t *testing.T) {
	if got := load(t).NormalizeVersion; got != Version {
		t.Fatalf("fixture was produced by normalize version %q, this package is %q -- "+
			"regenerate it with 'python eval/conformance.py' and treat the diff as a backfill",
			got, Version)
	}
}

func TestDescriptionMatchesTheReference(t *testing.T) {
	for _, c := range load(t).Descriptions {
		if got := Description(c.In); got != c.Out {
			t.Errorf("Description(%q)\n got %q\nwant %q", c.In, got, c.Out)
		}
	}
}

func TestCounterpartyKeyMatchesTheReference(t *testing.T) {
	for _, c := range load(t).Keys {
		key, tier := CounterpartyKey(c.Name, c.TaxID, c.Account)
		if key != c.Key || string(tier) != c.Tier {
			t.Errorf("CounterpartyKey(%q, %q, %q)\n got (%q, %q)\nwant (%q, %q)",
				c.Name, c.TaxID, c.Account, key, tier, c.Key, c.Tier)
		}
	}
}

// Task 4.4's other half. Version is not a decoration on the two functions: it
// is the reason a stored description_norm can be trusted six months later, so
// it has to be reachable from outside the package and has to be the string
// the fixture was produced under.
func TestVersionIsExported(t *testing.T) {
	if Version == "" {
		t.Fatal("Version is empty, so nothing stored beside it says what produced it")
	}
}
