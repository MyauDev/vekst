package ingest

import "encoding/json"

// reportSchema versions report_jsonb's shape (design D5), independent of
// migration numbers: a reader can tell which shape it is looking at without
// cross-referencing which schema version was live when a row was written.
const reportSchema = 1

// maxReportedErrors and maxRawLength bound the stored report so a hostile or
// broken file cannot produce a report larger than the file itself (design
// risk table). error_count -- the table's own column, always the true
// total -- is what a client compares against len(errors) to know whether
// anything was cut.
const (
	maxReportedErrors = 100
	maxRawLength      = 200
)

type reportError struct {
	Line  int    `json:"line"`
	Field string `json:"field"`
	Code  string `json:"code"`
	Raw   string `json:"raw"`
}

type reportWarning struct {
	Code   string                 `json:"code"`
	Detail *BalanceMismatchDetail `json:"detail,omitempty"`
}

type reportDoc struct {
	Schema   int             `json:"schema"`
	Errors   []reportError   `json:"errors"`
	Warnings []reportWarning `json:"warnings"`
}

// BuildReportJSON renders a ValidationResult into report_jsonb's committed
// shape (design D5). No value in it is ever a rendered sentence (CLAUDE.md);
// TestReportJSONCarriesNoSentences asserts that structurally rather than by
// convention.
func BuildReportJSON(result ValidationResult) ([]byte, error) {
	doc := reportDoc{Schema: reportSchema}

	n := len(result.Errors)
	if n > maxReportedErrors {
		n = maxReportedErrors
	}
	for _, e := range result.Errors[:n] {
		doc.Errors = append(doc.Errors, reportError{
			Line: e.Line, Field: e.Field, Code: e.Code, Raw: capRaw(e.Raw),
		})
	}

	for _, w := range result.Warnings {
		doc.Warnings = append(doc.Warnings, reportWarning{Code: w.Code, Detail: w.Balance})
	}

	return json.Marshal(doc)
}

// ParseReportJSON is BuildReportJSON's inverse, for GetValidationReport to
// read a stored row back into the same Go shapes the RPC translates from.
func ParseReportJSON(raw []byte) (errs []ValidationError, warns []ValidationWarning, err error) {
	var doc reportDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	for _, e := range doc.Errors {
		errs = append(errs, ValidationError{Line: e.Line, Field: e.Field, Code: e.Code, Raw: e.Raw})
	}
	for _, w := range doc.Warnings {
		warns = append(warns, ValidationWarning{Code: w.Code, Balance: w.Detail})
	}
	return errs, warns, nil
}

func capRaw(s string) string {
	if len(s) > maxRawLength {
		return s[:maxRawLength]
	}
	return s
}
