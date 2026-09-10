package ingest

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/MyauDev/vekst/core/internal/money"
)

// DecimalFromLocale turns a bank's rendering of a number into a plain decimal
// string that money.Parse accepts.
//
// CIS exports write "2 009,07": a space, sometimes a non-breaking space,
// between thousands, and a comma for the decimal point. Some write "2.009,07".
// Both mean the same amount, and neither parses as a Go float -- which is the
// point: the value goes to money.Parse and becomes int64 minor units, scaled by
// the currency's own exponent. It never passes through a float64 on the way.
func DecimalFromLocale(s string) (string, error) {
	s = strings.TrimSpace(strings.NewReplacer(" ", "", " ", "", " ", "").Replace(s))
	if s == "" {
		return "0", nil
	}

	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")

	comma, dot := strings.LastIndex(s, ","), strings.LastIndex(s, ".")
	switch {
	case comma >= 0 && dot >= 0:
		// Whichever comes last is the decimal separator; the other groups
		// thousands. "2.009,07" and "2,009.07" are both unambiguous this way.
		if comma > dot {
			s = strings.ReplaceAll(s[:comma], ".", "") + "." + s[comma+1:]
		} else {
			s = strings.ReplaceAll(s[:dot], ",", "") + "." + s[dot+1:]
		}
	case comma >= 0:
		s = s[:comma] + "." + s[comma+1:]
	}

	if !plainDecimal.MatchString(s) {
		return "", fmt.Errorf("ingest: %q is not a number", s)
	}
	if neg {
		s = "-" + s
	}
	return s, nil
}

var plainDecimal = regexp.MustCompile(`^\d+(\.\d+)?$`)

// ParseAmount reads a bank-formatted amount into Money.
func ParseAmount(currencyCode, raw string) (money.Money, error) {
	decimal, err := DecimalFromLocale(raw)
	if err != nil {
		return money.Money{}, err
	}
	return money.Parse(currencyCode, decimal)
}

// dateLayouts are tried in order. DD.MM.YYYY comes first because every CIS
// export in `core/testdata` uses it, and because "08.01.2024" is the 8th of
// January there and would be the 1st of August under a US layout -- a silent
// seven-month error, not a parse failure.
var dateLayouts = []string{
	"02.01.2006",
	"2006-01-02",
	"02/01/2006",
	"02-01-2006",
}

// ParseDate reads a statement date. It returns a UTC date with no time
// component: a booking date is a day, not an instant, and giving it a spurious
// midnight in some local zone is how a transaction moves between months.
func ParseDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	for _, layout := range dateLayouts {
		if t, err := time.ParseInLocation(layout, raw, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("ingest: %q is not a date in any known layout", raw)
}
