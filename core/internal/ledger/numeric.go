package ledger

import (
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// numericFromDecimalString parses an exact decimal string -- never a float
// -- into the wire type. pgtype.Numeric.Scan reads plain decimal text
// directly, which is what makes this a type change rather than an
// arithmetic one.
func numericFromDecimalString(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, err
	}
	return n, nil
}

// decimalStringFromNumeric is the reverse: MarshalJSON renders pgtype's own
// big.Int-and-exponent pair as exact decimal text, with no float in
// between. "" for an unset (NULL) value.
func decimalStringFromNumeric(n pgtype.Numeric) (string, error) {
	if !n.Valid {
		return "", nil
	}
	b, err := n.MarshalJSON()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// numericFromConfidence and confidenceFromNumeric convert Classification's
// Confidence, a probability rather than money, for which float64 is exactly
// the right type -- the same choice vekst.internal.v1's own confidence
// field makes. numeric(4,3) rounds anything past three decimal places at
// write time regardless of what is sent, so formatting with no fixed
// precision here loses nothing the column would have kept anyway.
func numericFromConfidence(f float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(strconv.FormatFloat(f, 'f', -1, 64)); err != nil {
		return pgtype.Numeric{}, err
	}
	return n, nil
}

func confidenceFromNumeric(n pgtype.Numeric) (float64, error) {
	v, err := n.Float64Value()
	if err != nil {
		return 0, err
	}
	return v.Float64, nil
}
