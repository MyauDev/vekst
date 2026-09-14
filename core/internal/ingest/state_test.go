package ingest

import (
	"errors"
	"testing"
)

// allStatuses is independent of the transitions table in state.go, so this
// test can catch a bug in that table rather than confirming it against
// itself.
var allStatuses = []Status{
	StatusAwaitingUpload, StatusAbandoned, StatusUploaded,
	StatusParsing, StatusParsed, StatusValidating, StatusValidated, StatusRejected,
	StatusPersisting, StatusImported, StatusFailed,
}

// legalTransitions is design D3's pipeline, written out by hand as a second,
// independent source of truth for what task 6.9 calls "table-driven": every
// pair in this list must be accepted, and -- because it is exhaustive -- every
// pair not in it must be refused.
var legalTransitions = map[[2]Status]bool{
	{StatusAwaitingUpload, StatusUploaded}:  true,
	{StatusAwaitingUpload, StatusAbandoned}: true,
	{StatusAwaitingUpload, StatusFailed}:    true,
	{StatusUploaded, StatusParsing}:         true,
	{StatusUploaded, StatusFailed}:          true,
	{StatusParsing, StatusParsed}:           true,
	{StatusParsing, StatusFailed}:           true,
	{StatusParsed, StatusValidating}:        true,
	{StatusParsed, StatusFailed}:            true,
	{StatusValidating, StatusValidated}:     true,
	{StatusValidating, StatusRejected}:      true,
	{StatusValidating, StatusFailed}:        true,
	{StatusValidated, StatusPersisting}:     true,
	{StatusValidated, StatusFailed}:         true,
	{StatusPersisting, StatusImported}:      true,
	{StatusPersisting, StatusFailed}:        true,
}

func TestCheckTransitionTableDriven(t *testing.T) {
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			want := legalTransitions[[2]Status{from, to}]
			err := CheckTransition(from, to)

			if want && err != nil {
				t.Errorf("CheckTransition(%s, %s) = %v, want nil (legal transition)", from, to, err)
			}
			if !want && err == nil {
				t.Errorf("CheckTransition(%s, %s) = nil, want a refusal", from, to)
			}
			if err != nil {
				var illegal *ErrIllegalTransition
				if !errors.As(err, &illegal) {
					t.Errorf("CheckTransition(%s, %s) error is %T, want *ErrIllegalTransition", from, to, err)
				}
			}
		}
	}
}

func TestTerminalStatesHaveNoOutgoingTransition(t *testing.T) {
	for _, terminal := range []Status{StatusAbandoned, StatusRejected, StatusImported, StatusFailed} {
		for _, to := range allStatuses {
			if err := CheckTransition(terminal, to); err == nil {
				t.Errorf("CheckTransition(%s, %s) = nil, want a refusal: %s is terminal", terminal, to, terminal)
			}
		}
	}
}

func TestRejectedAndFailedStayDistinct(t *testing.T) {
	// A validation outcome (2.3's job) must never fall through to the
	// defect-in-us state, and the reverse. Collapsing them would show a
	// customer their own file's problem as if it were ours, or vice versa.
	if err := CheckTransition(StatusValidating, StatusRejected); err != nil {
		t.Errorf("validating -> rejected must be legal: %v", err)
	}
	if err := CheckTransition(StatusRejected, StatusFailed); err == nil {
		t.Error("rejected -> failed must be refused: rejected is terminal")
	}
	if err := CheckTransition(StatusFailed, StatusRejected); err == nil {
		t.Error("failed -> rejected must be refused: failed is terminal")
	}
}
