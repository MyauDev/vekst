package ingest

import (
	"encoding/json"
	"strings"
	"testing"
)

// A malformed file cannot produce an unbounded report (design risk table,
// spec scenario "A malformed file cannot produce an unbounded report"): the
// stored errors array is capped, independently of error_count -- which
// validatejob.go computes from the full, untruncated list -- so a client can
// still tell "150 errors, 100 shown" apart from "100 errors, 100 shown".
func TestBuildReportJSONTruncatesALargeErrorList(t *testing.T) {
	result := ValidationResult{Outcome: OutcomeRejected}
	for i := 0; i < 150; i++ {
		result.Errors = append(result.Errors, ValidationError{Line: i + 1, Field: "debit", Code: CodeAmountUnparseable, Raw: "x"})
	}

	raw, err := BuildReportJSON(result)
	if err != nil {
		t.Fatalf("BuildReportJSON: %v", err)
	}
	var doc reportDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshaling report: %v", err)
	}
	if len(doc.Errors) != maxReportedErrors {
		t.Errorf("stored error count = %d, want %d (capped)", len(doc.Errors), maxReportedErrors)
	}
	// The true total is what the caller passes to InsertValidation's own
	// error_count column -- computed from the full list, not this capped one.
	if len(result.Errors) != 150 {
		t.Errorf("the full error list itself must stay untruncated: got %d, want 150", len(result.Errors))
	}
}

func TestBuildReportJSONCapsRawText(t *testing.T) {
	longRaw := strings.Repeat("x", maxRawLength+50)
	result := ValidationResult{Outcome: OutcomeRejected, Errors: []ValidationError{
		{Line: 1, Field: "description", Code: CodeReplacementCharacter, Raw: longRaw},
	}}

	raw, err := BuildReportJSON(result)
	if err != nil {
		t.Fatalf("BuildReportJSON: %v", err)
	}
	var doc reportDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshaling report: %v", err)
	}
	if len(doc.Errors[0].Raw) != maxRawLength {
		t.Errorf("stored raw length = %d, want %d (capped)", len(doc.Errors[0].Raw), maxRawLength)
	}
}

func TestParseReportJSONRoundTrips(t *testing.T) {
	result := ValidationResult{
		Outcome: OutcomeValidWithWarnings,
		Errors:  []ValidationError{{Line: 5, Field: "debit", Code: CodeAmountUnparseable, Raw: "bad"}},
		Warnings: []ValidationWarning{
			{Code: CodeBalanceMismatch, Balance: &BalanceMismatchDetail{Opening: 100, Movements: 50, Closing: 151, Difference: -1, Currency: "EUR"}},
			{Code: CodePeriodGap},
		},
	}
	raw, err := BuildReportJSON(result)
	if err != nil {
		t.Fatalf("BuildReportJSON: %v", err)
	}
	errs, warns, err := ParseReportJSON(raw)
	if err != nil {
		t.Fatalf("ParseReportJSON: %v", err)
	}
	if len(errs) != 1 || errs[0].Code != CodeAmountUnparseable || errs[0].Line != 5 {
		t.Errorf("errors round-tripped as %+v", errs)
	}
	if len(warns) != 2 {
		t.Fatalf("warnings round-tripped as %+v", warns)
	}
	if warns[0].Balance == nil || warns[0].Balance.Difference != -1 {
		t.Errorf("balance detail round-tripped as %+v", warns[0].Balance)
	}
	if warns[1].Balance != nil {
		t.Errorf("period_gap warning should carry no detail, got %+v", warns[1].Balance)
	}
}
