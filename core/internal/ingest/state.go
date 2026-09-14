package ingest

import "fmt"

// Status is one of the eleven values import_batches.status admits (migration
// 008, design D3). The whole pipeline is declared here now, even though this
// change only ever drives three of the transitions below -- 2.2, 2.3 and 2.5
// drive the rest. The alternative is a migration per change that each widens
// the same CHECK constraint, which is three chances for two of them to
// disagree about what it says.
type Status string

const (
	StatusAwaitingUpload Status = "awaiting_upload"
	StatusAbandoned      Status = "abandoned"
	StatusUploaded       Status = "uploaded"
	StatusParsing        Status = "parsing"
	StatusParsed         Status = "parsed"
	StatusValidating     Status = "validating"
	StatusValidated      Status = "validated"
	StatusRejected       Status = "rejected"
	StatusPersisting     Status = "persisting"
	StatusImported       Status = "imported"
	StatusFailed         Status = "failed"
)

// transitions is the whole pipeline, drawn in ARCHITECTURE.md §4a and fixed
// there:
//
//	awaiting_upload ─┬─▶ uploaded ─▶ parsing ─▶ parsed ─▶ validating ─┬─▶ validated ─▶ persisting ─▶ imported
//	                 │                                                └─▶ rejected
//	                 └─▶ abandoned                    any stage ──────────▶ failed
//
// Enforced here, in Go, rather than in a database trigger: the legal next
// state depends on which stage is calling, which a CHECK constraint or a
// trigger has no way to see. rejected and failed stay apart deliberately --
// rejected is a validation outcome shown to the customer as their file's
// problem, failed is a defect in us -- so this table does not let one collapse
// into the other.
var transitions = map[Status]map[Status]bool{
	StatusAwaitingUpload: {StatusUploaded: true, StatusAbandoned: true, StatusFailed: true},
	StatusUploaded:       {StatusParsing: true, StatusFailed: true},
	StatusParsing:        {StatusParsed: true, StatusFailed: true},
	StatusParsed:         {StatusValidating: true, StatusFailed: true},
	StatusValidating:     {StatusValidated: true, StatusRejected: true, StatusFailed: true},
	StatusValidated:      {StatusPersisting: true, StatusFailed: true},
	StatusPersisting:     {StatusImported: true, StatusFailed: true},
	// Terminal states. No outgoing edge, including to themselves: a second
	// failure of an already-failed batch is not a transition, it is a bug in
	// the caller that thinks it still owns the row.
	StatusAbandoned: {},
	StatusRejected:  {},
	StatusImported:  {},
	StatusFailed:    {},
}

// ErrIllegalTransition names the two states an attempted move could not
// connect.
type ErrIllegalTransition struct {
	From, To Status
}

func (e *ErrIllegalTransition) Error() string {
	return fmt.Sprintf("ingest: %s -> %s is not a legal transition", e.From, e.To)
}

// CheckTransition reports whether moving a batch from `from` to `to` is
// legal, returning *ErrIllegalTransition if not. It is a pure function: no
// database handle, no clock, the same shape as this package's parsers -- a
// stage decides its own next state, and the table above is the only thing it
// consults.
func CheckTransition(from, to Status) error {
	if transitions[from][to] {
		return nil
	}
	return &ErrIllegalTransition{From: from, To: to}
}
