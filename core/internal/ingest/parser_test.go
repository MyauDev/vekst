package ingest_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/MyauDev/vekst/core/internal/ingest"
)

// The point of the registry: a caller hands over bytes and does not name a
// bank. Adding a second bank must not change this call site.
func TestParseDetectsTheFormat(t *testing.T) {
	for _, path := range fixtures(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		st, err := ingest.Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if st.Format != "priorbank-by" {
			t.Errorf("%s: detected as %q", path, st.Format)
		}
		if len(st.Rows) == 0 {
			t.Errorf("%s: detected but produced no rows", path)
		}
	}
}

// An unrecognised file must say so plainly, and say what was tried. "Unsupported
// file" on its own tells a customer nothing they can act on.
func TestUnknownFormatNamesWhatWasTried(t *testing.T) {
	_, err := ingest.Parse([]byte("date,amount\n2024-01-01,10.00\n"))
	if err == nil {
		t.Fatal("a plain CSV was accepted by some parser")
	}
	var unknown *ingest.ErrUnknownFormat
	if !errors.As(err, &unknown) {
		t.Fatalf("want ErrUnknownFormat, got %T: %v", err, err)
	}
	if len(unknown.Tried) == 0 {
		t.Error("the error should list the formats that were tried")
	}
	for _, name := range unknown.Tried {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error text omits %q", name)
		}
	}
}

// Detection must be certain, not eager. A parser that claims a file it cannot
// read turns a clear "we do not recognise this" into a parse failure halfway
// down, and the customer is told the wrong thing about their own data.
func TestDetectionDoesNotClaimForeignFiles(t *testing.T) {
	notPriorbank := [][]byte{
		[]byte("Дата док.;N док.;Назначение\n01.01.2024;1;тест\n"), // right columns, wrong bank
		[]byte("Приорбанк платёж за обслуживание счёта\n"),         // the name, but no statement
		[]byte(""),
		[]byte("\x00\x01\x02binary"),
	}
	for i, raw := range notPriorbank {
		if _, err := ingest.ParserFor(raw); err == nil {
			t.Errorf("case %d: some parser claimed a file it should not recognise", i)
		}
	}
}

func TestFormatsAreListed(t *testing.T) {
	formats := ingest.Formats()
	if len(formats) == 0 {
		t.Fatal("no parser is registered")
	}
	var found bool
	for _, f := range formats {
		if f == "priorbank-by" {
			found = true
		}
	}
	if !found {
		t.Errorf("priorbank-by is not registered; got %v", formats)
	}
}
