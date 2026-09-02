package money

import "testing"

// The exponent table is data, not a generated artefact, so it is covered by
// a test rather than the codegen drift gate (design D5/Q4).
func TestExponentKnownEntries(t *testing.T) {
	cases := map[string]int{
		"EUR": 2, // the common case
		"USD": 2,
		"JPY": 0, // no minor unit at all
		"KRW": 0,
		"KWD": 3, // three fils digits
		"BHD": 3,
		"CLF": 4, // the rare four-digit case
	}
	for code, want := range cases {
		got, ok := Exponent(code)
		if !ok {
			t.Fatalf("Exponent(%q): not found", code)
		}
		if got != want {
			t.Fatalf("Exponent(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestExponentUnknownCodeIsNotFound(t *testing.T) {
	if _, ok := Exponent("ZZZ"); ok {
		t.Fatal("Exponent(ZZZ): want not found, ZZZ is not an ISO-4217 code")
	}
}
