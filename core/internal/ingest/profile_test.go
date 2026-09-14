package ingest

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
)

const syntheticPriorbankDoc = "Приорбанк Открытое акционерное общество, БИК PJCBBY2X;01.01.2026;\n" +
	"\n" +
	"Дата док.;N док.;Код опер;Корреспондент.Код;Корреспондент.Счет;Корреспондент.Название;Номинал.Дебет;Номинал.Кредит;Назначение;\n" +
	"01.01.2026;1;100;EUR;BY00TEST1;GOOD ROW;0,00;50,00;payment one;\n"

func allFixturePaths(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "priorbank-by", "*.csv"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no Priorbank fixtures found: %v", err)
	}
	return paths
}

// Task 6.1: no profile, no change. ParsePriorbank and ParsePriorbankWithParams
// with a nil params produce byte-identical statements -- checked by deep
// equality rather than asserted, since the two are meant to be indistinguishable.
func TestNoProfileNoChange(t *testing.T) {
	for _, path := range allFixturePaths(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		st1, err := ParsePriorbank(raw)
		if err != nil {
			t.Fatalf("%s: ParsePriorbank: %v", path, err)
		}
		st2, err := ParsePriorbankWithParams(raw, nil)
		if err != nil {
			t.Fatalf("%s: ParsePriorbankWithParams(nil): %v", path, err)
		}
		if !reflect.DeepEqual(st1, st2) {
			t.Errorf("%s: ParsePriorbank and ParsePriorbankWithParams(nil, ...) disagree", path)
		}
	}
}

// Task 6.2: a partial override -- naming only DateFormat -- leaves charset,
// delimiter and column resolution to the detector.
func TestPartialOverrideLeavesOtherFieldsToTheDetector(t *testing.T) {
	params := &Parameters{DateFormat: "02.01.2006"}
	st, err := ParsePriorbankWithParams([]byte(syntheticPriorbankDoc), params)
	if err != nil {
		t.Fatalf("ParsePriorbankWithParams: %v", err)
	}
	if len(st.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(st.Rows))
	}
	row := st.Rows[0]
	if row.CounterpartyName != "GOOD ROW" || row.Description != "payment one" {
		t.Errorf("column resolution changed under a DateFormat-only override: %+v", row)
	}
	if row.Credit.MinorUnits != 5000 {
		t.Errorf("Credit = %d, want 5000 -- decimal separator detection should be untouched", row.Credit.MinorUnits)
	}
}

// Task 6.3: a full override is actually applied. The design's own framing
// ("a profile naming a wrong charset produces a file that fails the U+FFFD
// correctness check") assumes decoding under the wrong charset always
// produces invalid UTF-8 -- verified against a real fixture that it does
// not, for windows-1251 versus CP866: both are complete 8-bit charmaps with
// no invalid byte sequence, so forcing one onto text written in the other
// produces different, but validly-decodable, wrong Cyrillic-like text
// rather than U+FFFD. What it reliably breaks instead is the header-row
// match ("Дата док" no longer decodes to itself), which is an equally
// strong and considerably more general proof that the override reached the
// parser: ParsePriorbankWithParams fails outright, on a file that parses
// cleanly under its own detected charset.
func TestFullOverrideChangesTheResult(t *testing.T) {
	fixture := realFixtureBody(t)

	if _, err := ParsePriorbankWithParams(fixture, nil); err != nil {
		t.Fatalf("test setup: the real fixture must parse under its own detected charset: %v", err)
	}

	detected := DetectCharset(fixture)
	wrong := CharsetCP866
	if detected == CharsetCP866 {
		wrong = CharsetWindows125
	}
	if _, err := ParsePriorbankWithParams(fixture, &Parameters{Charset: wrong}); err == nil {
		t.Error("forcing the wrong charset should break parsing, not silently succeed")
	}
}

// Task 6.6 (the Go half): the canonical field list matches migration 011's
// own array literal, so the two cannot drift silently. Reads the migration's
// SQL source directly rather than a live database, so this runs in plain
// `go test ./...` with no DATABASE_URL set, like any other pure test here.
func TestCanonicalFieldListMatchesTheMigrationsTrigger(t *testing.T) {
	migrationFields := fieldsFromMigrationTrigger(t)
	got := append([]string{}, CanonicalFields...)
	want := migrationFields
	sortStrings(got)
	sortStrings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CanonicalFields = %v\nmigration trigger names = %v", got, want)
	}
}

var fieldLiteral = regexp.MustCompile(`'([a-z_]+)'`)

func fieldsFromMigrationTrigger(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "migrations", "00011_import_profiles.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	src := string(raw)
	anchor := indexOf(src, "canonical_fields text[] := ARRAY[")
	if anchor < 0 {
		t.Fatal("migration 011 has no canonical_fields array literal")
	}
	// anchor + len(...) lands just past the literal's own opening "[" --
	// "text[]" earlier in the same line has a closing bracket of its own,
	// which would otherwise be the first "]" found.
	start := anchor + len("canonical_fields text[] := ARRAY[")
	end := indexOf(src[start:], "]")
	if end < 0 {
		t.Fatal("canonical_fields array literal has no closing bracket")
	}
	block := src[start : start+end]
	matches := fieldLiteral.FindAllStringSubmatch(block, -1)
	fields := make([]string, len(matches))
	for i, m := range matches {
		fields[i] = m[1]
	}
	return fields
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
