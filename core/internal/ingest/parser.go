package ingest

import (
	"fmt"
	"sort"
	"strings"
)

// Parser reads one bank's export format.
//
// There is a parser per bank, not per file extension, and that is the whole
// reason this interface exists. Two banks both writing CSV agree on almost
// nothing else: Priorbank puts a seven-line preamble above the header, changes
// its column set between rouble and currency accounts, and writes summary rows
// that ignore the header's columns entirely. A second bank will be wrong in its
// own way.
//
// Every implementation is a pure function over bytes. No database handle, no
// clock, no network — the same bytes always produce the same Statement, which
// is what lets a statement be re-parsed to answer a question about a report
// printed months ago.
type Parser interface {
	// Name identifies the format in errors, in an import profile, and on the
	// batch. Shaped as bank-country: "priorbank-by".
	Name() string

	// Detect reports whether these bytes look like this format. It must be
	// cheap and it must be certain: a parser that answers yes to a file it
	// cannot read turns a clear "we do not recognise this" into a confusing
	// parse failure halfway down.
	Detect(raw []byte) bool

	// Parse reads the whole file. An error names the line in the original
	// file, because somebody will go looking for it.
	Parse(raw []byte) (*Statement, error)
}

var registry []Parser

// Register adds a parser. Called from an init function in each parser's file,
// so adding a bank is adding a file and nothing else.
func Register(p Parser) {
	registry = append(registry, p)
	sort.Slice(registry, func(i, j int) bool { return registry[i].Name() < registry[j].Name() })
}

// Formats lists the registered parsers, for an error message and for the
// upload screen's "we can read these" list.
func Formats() []string {
	out := make([]string, 0, len(registry))
	for _, p := range registry {
		out = append(out, p.Name())
	}
	return out
}

// ErrUnknownFormat is returned when no parser recognises a file. It carries
// what was tried, because "unsupported file" on its own tells a customer
// nothing they can act on.
type ErrUnknownFormat struct {
	Tried []string
}

func (e *ErrUnknownFormat) Error() string {
	return fmt.Sprintf("ingest: no parser recognises this file; tried %s",
		strings.Join(e.Tried, ", "))
}

// ParserFor picks the parser for a file.
//
// Ambiguity is an error rather than a first-match win. Two parsers claiming one
// file means one of them is too eager, and finding that out here — with both
// names in the message — is cheaper than finding it out from a customer whose
// numbers came from the wrong reader.
func ParserFor(raw []byte) (Parser, error) {
	var claimed []Parser
	for _, p := range registry {
		if p.Detect(raw) {
			claimed = append(claimed, p)
		}
	}
	switch len(claimed) {
	case 0:
		return nil, &ErrUnknownFormat{Tried: Formats()}
	case 1:
		return claimed[0], nil
	default:
		names := make([]string, 0, len(claimed))
		for _, p := range claimed {
			names = append(names, p.Name())
		}
		return nil, fmt.Errorf("ingest: %d parsers claim this file (%s); one of them detects too loosely",
			len(claimed), strings.Join(names, ", "))
	}
}

// Parse detects the format and reads the file. This is the entry point every
// caller uses; the per-bank functions stay exported only so a test can pin one.
func Parse(raw []byte) (*Statement, error) {
	p, err := ParserFor(raw)
	if err != nil {
		return nil, err
	}
	st, err := p.Parse(raw)
	if err != nil {
		return nil, err
	}
	st.Format = p.Name()
	return st, nil
}
