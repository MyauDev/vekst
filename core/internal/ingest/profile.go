package ingest

import "encoding/json"

// CanonicalFields is the closed list column_map values may name (design D2,
// task 0.1). Every one of them has to land in a column of transactions
// (change 2.5), which is what makes the list closed rather than whatever a
// customer's own header happens to say.
//
// Defined once, here, in Go: migration 011's constraint trigger is generated
// from this exact list (task 1.3) rather than carrying its own copy, and
// TestCanonicalFieldListMatchesTheTrigger (task 6.6) fails the day the two
// would otherwise disagree.
var CanonicalFields = []string{
	"booked_on", "value_on", "amount", "debit", "credit", "currency",
	"description", "counterparty", "bank_ref", "document_ref", "posting_no",
	"opening_balance", "closing_balance", "row_count_declared",
}

// IsCanonicalField reports whether field is one of CanonicalFields.
func IsCanonicalField(field string) bool {
	for _, f := range CanonicalFields {
		if f == field {
			return true
		}
	}
	return false
}

// ColumnMap is a profile's source-column-name -> canonical-field mapping.
// Keys are the source file's own words and cannot be constrained; values are
// checked against CanonicalFields wherever a ColumnMap is written.
type ColumnMap map[string]string

// Parameters is what a profile can override, field by field (design D1): a
// zero value in any field means "let the detector decide that one" -- the
// same shape import_profiles' own columns have, all nullable except
// ColumnMap. There is no "partial" ColumnMap: a detector cannot guess which
// column is the amount, so a caller either supplies the whole map or none of
// it.
type Parameters struct {
	Charset    Charset // "" means detect (DetectCharset)
	Delimiter  byte    // 0 means the parser's own default
	DecimalSep string  // "" means detect by position (DecimalFromLocale); "," or "."
	DateFormat string  // "" means try this package's own known layouts (ParseDate)
	ColumnMap  ColumnMap
}

// resolvedParametersJSON is what a batch's resolved_parameters column
// stores (task 5.3 in add-import-profiles): what it was actually parsed
// with, independent of a profile that may be edited afterward. p is
// already the fully-resolved set validatejob.go and persistjob.go both
// parsed with, so this snapshots p directly rather than re-reading the
// profile row a second time.
func (p *Parameters) resolvedParametersJSON() ([]byte, error) {
	if p == nil {
		return nil, nil
	}
	delim := ""
	if p.Delimiter != 0 {
		delim = string(p.Delimiter)
	}
	return json.Marshal(resolvedParameters{
		Charset:    string(p.Charset),
		Delimiter:  delim,
		DecimalSep: p.DecimalSep,
		DateFormat: p.DateFormat,
		ColumnMap:  p.ColumnMap,
	})
}

// sourceColumn resolves which header name canonical field should be read
// from: the profile's override if p names one, otherwise def -- the
// parser's own built-in name. A nil Parameters (no profile at all) always
// returns def, which is what makes "no profile, no change" (task 6.1) true
// by construction rather than by a separate code path.
func (p *Parameters) sourceColumn(field, def string) string {
	if p == nil || p.ColumnMap == nil {
		return def
	}
	for source, canonical := range p.ColumnMap {
		if canonical == field {
			return source
		}
	}
	return def
}
