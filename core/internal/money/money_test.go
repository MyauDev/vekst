package money

import (
	"errors"
	"testing"
)

func TestNewRejectsUnknownCurrency(t *testing.T) {
	if _, err := New("XXX-NOT-A-CODE", 100); !errors.Is(err, ErrUnknownCurrency) {
		t.Fatalf("New with an unknown code: got %v, want ErrUnknownCurrency", err)
	}
}

func TestAddRefusesMixedCurrency(t *testing.T) {
	eur := Money{CurrencyCode: "EUR", MinorUnits: 1000}
	pln := Money{CurrencyCode: "PLN", MinorUnits: 500}

	if _, err := Add(eur, pln); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Add(EUR, PLN): got %v, want ErrCurrencyMismatch", err)
	}
}

func TestAddSameCurrency(t *testing.T) {
	a := Money{CurrencyCode: "EUR", MinorUnits: 1234}
	b := Money{CurrencyCode: "EUR", MinorUnits: 66}

	got, err := Add(a, b)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	want := Money{CurrencyCode: "EUR", MinorUnits: 1300}
	if !Equal(got, want) {
		t.Fatalf("Add(1234, 66) = %+v, want %+v", got, want)
	}
}

// Currencies with non-standard exponents are handled -- neither is scaled by
// a hardcoded factor of 100 (JPY has exponent 0, KWD has exponent 3).
func TestFormatNonStandardExponents(t *testing.T) {
	cases := []struct {
		name string
		m    Money
		want string
	}{
		{"JPY whole yen, exponent 0", Money{"JPY", 1234}, "1234"},
		{"KWD three fils digits", Money{"KWD", 1234}, "1.234"},
		{"EUR two cent digits", Money{"EUR", 1234}, "12.34"},
		{"negative EUR", Money{"EUR", -1234}, "-12.34"},
		{"KWD exact thousand", Money{"KWD", 1000}, "1.000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Format(tc.m)
			if err != nil {
				t.Fatalf("Format(%+v): %v", tc.m, err)
			}
			if got != tc.want {
				t.Fatalf("Format(%+v) = %q, want %q", tc.m, got, tc.want)
			}
		})
	}
}

func TestParseNonStandardExponents(t *testing.T) {
	cases := []struct {
		name     string
		currency string
		decimal  string
		want     int64
	}{
		{"JPY has no minor units", "JPY", "1234", 1234},
		{"KWD three fils digits", "KWD", "1.234", 1234},
		{"KWD accepts fewer digits, right-padded", "KWD", "1.2", 1200},
		{"EUR two cent digits", "EUR", "12.34", 1234},
		{"negative EUR", "EUR", "-12.34", -1234},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.currency, tc.decimal)
			if err != nil {
				t.Fatalf("Parse(%q, %q): %v", tc.currency, tc.decimal, err)
			}
			if got.MinorUnits != tc.want || got.CurrencyCode != tc.currency {
				t.Fatalf("Parse(%q, %q) = %+v, want {%s %d}", tc.currency, tc.decimal, got, tc.currency, tc.want)
			}
		})
	}
}

func TestParseRejectsTooManyFractionDigits(t *testing.T) {
	if _, err := Parse("EUR", "1.234"); err == nil {
		t.Fatal("Parse(EUR, 1.234): want an error, EUR only has two minor-unit digits")
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	for _, m := range []Money{{"EUR", 12345}, {"JPY", 999}, {"KWD", 1}, {"EUR", -1}} {
		s, err := Format(m)
		if err != nil {
			t.Fatalf("Format(%+v): %v", m, err)
		}
		got, err := Parse(m.CurrencyCode, s)
		if err != nil {
			t.Fatalf("Parse(%q, %q): %v", m.CurrencyCode, s, err)
		}
		if !Equal(got, m) {
			t.Fatalf("round trip %+v -> %q -> %+v", m, s, got)
		}
	}
}
