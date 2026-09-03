// Package money represents monetary amounts as integer minor units plus an
// ISO-4217 currency code, and never as a floating-point number -- in this
// package or anywhere a money field exists in the codebase. See
// ARCHITECTURE.md §6 and openspec design D5.
package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	// ErrUnknownCurrency means the code is not in the checked-in ISO-4217
	// exponent table. Never defaulted to an assumed exponent.
	ErrUnknownCurrency = errors.New("money: unknown currency code")

	// ErrCurrencyMismatch means an operation was asked to combine two
	// different currencies. It never silently picks one.
	ErrCurrencyMismatch = errors.New("money: currency codes do not match")
)

// Money is a signed amount in a currency's minor units, together with the
// ISO-4217 code needed to interpret it.
type Money struct {
	CurrencyCode string
	MinorUnits   int64
}

// New validates currencyCode against the exponent table before returning a
// Money. An unknown code is an error, never a guess.
func New(currencyCode string, minorUnits int64) (Money, error) {
	if _, ok := Exponent(currencyCode); !ok {
		return Money{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, currencyCode)
	}
	return Money{CurrencyCode: currencyCode, MinorUnits: minorUnits}, nil
}

// Add returns a + b. It refuses to operate on two different currencies
// rather than silently picking one.
func Add(a, b Money) (Money, error) {
	if a.CurrencyCode != b.CurrencyCode {
		return Money{}, fmt.Errorf("%w: %q vs %q", ErrCurrencyMismatch, a.CurrencyCode, b.CurrencyCode)
	}
	return Money{CurrencyCode: a.CurrencyCode, MinorUnits: a.MinorUnits + b.MinorUnits}, nil
}

// Equal reports whether a and b are the same amount in the same currency.
func Equal(a, b Money) bool {
	return a.CurrencyCode == b.CurrencyCode && a.MinorUnits == b.MinorUnits
}

// Format renders m as a decimal string, scaled by its currency's exponent
// from the table -- never by a hardcoded factor of 100. JPY (exponent 0)
// renders with no decimal point; KWD (exponent 3) renders with three digits.
func Format(m Money) (string, error) {
	exp, ok := Exponent(m.CurrencyCode)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownCurrency, m.CurrencyCode)
	}
	if exp == 0 {
		return strconv.FormatInt(m.MinorUnits, 10), nil
	}

	neg := m.MinorUnits < 0
	units := m.MinorUnits
	if neg {
		units = -units
	}
	scale := pow10(exp)
	whole, frac := units/scale, units%scale

	s := fmt.Sprintf("%d.%0*d", whole, exp, frac)
	if neg {
		s = "-" + s
	}
	return s, nil
}

// Parse is Format's inverse: a decimal string plus a currency code becomes a
// Money in that currency's minor units, scaled by its exponent from the
// table rather than a hardcoded 100.
func Parse(currencyCode, decimal string) (Money, error) {
	exp, ok := Exponent(currencyCode)
	if !ok {
		return Money{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, currencyCode)
	}

	neg := strings.HasPrefix(decimal, "-")
	s := strings.TrimPrefix(decimal, "-")
	whole, frac, hasFrac := strings.Cut(s, ".")

	if exp == 0 {
		if hasFrac {
			return Money{}, fmt.Errorf("money: %s has no minor units, got %q", currencyCode, decimal)
		}
		n, err := strconv.ParseInt(whole, 10, 64)
		if err != nil {
			return Money{}, fmt.Errorf("money: %q: %w", decimal, err)
		}
		return Money{CurrencyCode: currencyCode, MinorUnits: negIf(neg, n)}, nil
	}

	if len(frac) > exp {
		return Money{}, fmt.Errorf("money: %s takes %d minor-unit digits, got %q", currencyCode, exp, decimal)
	}
	frac += strings.Repeat("0", exp-len(frac))

	wholeN, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return Money{}, fmt.Errorf("money: %q: %w", decimal, err)
	}
	fracN, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return Money{}, fmt.Errorf("money: %q: %w", decimal, err)
	}

	return Money{CurrencyCode: currencyCode, MinorUnits: negIf(neg, wholeN*pow10(exp)+fracN)}, nil
}

func pow10(exp int) int64 {
	n := int64(1)
	for i := 0; i < exp; i++ {
		n *= 10
	}
	return n
}

func negIf(neg bool, n int64) int64 {
	if neg {
		return -n
	}
	return n
}
