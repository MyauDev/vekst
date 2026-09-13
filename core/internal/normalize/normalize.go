// Package normalize turns a bank's free text into something two sides can
// compare.
//
// It is a port of eval/norm.py, and the port is deliberate rather than
// incidental. Two callers need these functions and they are in different
// languages: the classifier matches a rule's stored text against a
// transaction's, and Track A's dedup_hash is computed in core before the
// classifier is ever called. Running normalisation in only one of them would
// mean core hashing one string and the classifier matching another.
//
// Because there are two implementations, one of them has to be the reference.
// eval/norm.py is, because it is what the harness measured the rule set with,
// and the accuracy numbers in the design are only true of the text it
// produced. conformance_test.go replays its output and fails on any
// difference -- including a difference this file introduced on purpose, which
// is the point: an improvement here changes what counts as a match, and that
// is a version bump and a backfill, not an edit.
package normalize

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

// Version is stored beside every value these functions produce. Changing what
// either function returns without changing this string reinterprets rows a
// customer already approved.
const Version = "v1"

// Bank boilerplate. It carries no meaning and breaks otherwise-identical
// matches. Every entry was found in a real statement, not imagined:
//
//	"Частичная оплата."     Priorbank, when it takes a fee in instalments
//	"Rachunek kontrahenta:" PKO BP, followed by the counterparty account
//	"/ROC/" "/PURP/"        SWIFT tags on cross-border payments
//
// Plain strings rather than patterns: the reference spells them as regular
// expressions, but not one of them contains a metacharacter beyond an escaped
// full stop, so a literal replacement is the same function with less to go
// wrong. They are matched after uppercasing, which is why they are written
// uppercase.
var noise = []string{
	"ЧАСТИЧНАЯ ОПЛАТА.",
	"RACHUNEK KONTRAHENTA:",
	"TYTUL:",
	"TYTUŁ:",
	"/ROC/",
	"/PURP/",
	"/URI/",
}

var quotes = strings.NewReplacer(
	"«", `"`, "»", `"`, "“", `"`, "”", `"`, "„", `"`,
	"‘", "'", "’", "'", " ", " ",
)

// upper is Unicode's full uppercase mapping, not Go's default simple one.
// strings.ToUpper leaves "ß" alone where Python's str.upper() produces "SS",
// and a German counterparty is exactly the kind of row where the two
// implementations would quietly disagree.
var upper = cases.Upper(language.Und)

// Description is the canonical form of a payment purpose, for matching rules
// against it. Both the rule's stored value and the transaction's text go
// through it, so a bank prefix present on one side and absent on the other
// stops mattering.
func Description(s string) string {
	if s == "" {
		return ""
	}
	// NFKC first: files exported on macOS arrive decomposed, so "й" is two
	// code points there and one here. Without this they never compare equal.
	s = upper.String(quotes.Replace(norm.NFKC.String(s)))
	for _, n := range noise {
		s = strings.ReplaceAll(s, n, " ")
	}
	return collapseSpaces(joinDigitGroups(s))
}

// joinDigitGroups turns "03 5954 0014 67" into "035954001467". A Polish
// counterparty account is written three ways in one export; without this each
// spelling needs its own rule, and eighteen of them did.
//
// Written out rather than expressed as a pattern because the reference uses
// look-around, which RE2 does not have -- and which would not be clearer here
// if it did.
func joinDigitGroups(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if !isSeparator(runes[i]) || i == 0 || !unicode.IsDigit(runes[i-1]) {
			b.WriteRune(runes[i])
			continue
		}
		// A run of separators falls away only when a digit follows it as
		// well. "12 - 34" joins; "12 - AB" keeps its spaces.
		j := i
		for j < len(runes) && isSeparator(runes[j]) {
			j++
		}
		if j < len(runes) && unicode.IsDigit(runes[j]) {
			i = j - 1
			continue
		}
		for ; i < j; i++ {
			b.WriteRune(runes[i])
		}
		i--
	}
	return b.String()
}

func isSeparator(r rune) bool { return unicode.IsSpace(r) || r == '-' }

func collapseSpaces(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

// Legal forms, stripped so that "ООО РОМАШКА" and "РОМАШКА ООО" are one
// counterparty. Order is load-bearing and matches the reference exactly: a
// form is tried only once every form written before it has failed, so "ТОВ"
// is tried before "ТОВАРИЩЕСТВО С ОГРАНИЧЕННОЙ ОТВЕТСТВЕННОСТЬЮ" and fails on
// the word boundary rather than truncating it.
var legalForms = []string{
	"ООО", "ОДО", "ЗАО", "ОАО", "УП", "ЧУП", "ИП", "ТОО", "АО", "ТОВ", "ПАО",
	"ИНДИВИДУАЛЬНЫЙ ПРЕДПРИНИМАТЕЛЬ",
	"ТОВАРИЩЕСТВО С ОГРАНИЧЕННОЙ ОТВЕТСТВЕННОСТЬЮ",
	"LLC", "LLP", "LTD", "LIMITED", `INC\.?`, `CORP\.?`, "GMBH", "AB", "OU",
	"OÜ", "SARL", "SRL", "PTY", "BV", "NV", "SA", "AG", "KG",
	"SPOLKA Z OGRANICZONA ODPOWIEDZIALNOSCIA",
	"SPÓŁKA Z OGRANICZONĄ ODPOWIEDZIALNOŚCIĄ",
	`SP\.?\s?Z\.?\s?O\.?\s?O\.?`,
	`S\.A\.`,
}

// Each form carries its own trailing boundary. The reference writes that
// boundary as a look-ahead, so that the whitespace after one form stays
// available to the next; RE2 has no look-ahead, so the capture group records
// where the form itself ended and the whitespace is simply not consumed.
var legalFormRE = func() []*regexp.Regexp {
	res := make([]*regexp.Regexp, len(legalForms))
	for i, f := range legalForms {
		res[i] = regexp.MustCompile(`^((?:` + f + `))(?:\s|$)`)
	}
	return res
}()

// formEnd returns the byte index just past a legal form starting at at, or
// -1. Forms are tried in the order they are written, which is the order the
// reference tries them in.
func formEnd(s string, at int) int {
	for _, re := range legalFormRE {
		if m := re.FindStringSubmatchIndex(s[at:]); m != nil {
			return at + m[3]
		}
	}
	return -1
}

// stripLegalForms removes every legal form that stands as its own word.
//
// A form begins either at the start of the string or after whitespace, and
// that whitespace is part of what is removed. Both are tried at every
// position, in that order: the reference writes them as an alternation, so a
// string that opens with a space gets the start-of-string branch attempted,
// rejected, and then the whitespace branch -- and a port that tries only one
// of them silently keeps the form.
func stripLegalForms(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		end := -1
		if i == 0 {
			end = formEnd(s, 0)
		}
		if end < 0 && unicode.IsSpace(r) {
			end = formEnd(s, i+w)
		}
		if end < 0 {
			b.WriteRune(r)
			i += w
			continue
		}
		b.WriteByte(' ')
		i = end
	}
	return b.String()
}

// Start-of-address markers. Some banks (PKO BP) return the name and the
// address in one field, and an address is reformatted far more often than a
// counterparty changes. "ARDRESS" is not a typo here -- it is a typo in the
// source data.
var addressMarkers = []string{
	"ADDRESS", "ADRESS", "ARDRESS", "ADRES", "UL.", "STR.", "ST.", "PLAZA", "ROAD",
}

// findAddress returns the byte offset where the address begins, or -1.
//
// Three ways an address announces itself, tried at every position from the
// left: a marker word, a comma, or a run of two or more digits after a space
// -- a house or postal number. The word boundary after a marker is spelled
// out rather than left to RE2's \b, which is ASCII-only: a Cyrillic letter
// after "ROAD" is a word character to the reference and would not be to the
// pattern.
func findAddress(s string) int {
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])

		// A marker at the start of the string, or after this space. Both
		// are tried, for the reason stripLegalForms gives at length.
		if i == 0 && hasMarker(s) {
			return 0
		}
		if unicode.IsSpace(r) && hasMarker(s[i+w:]) {
			return i
		}
		if r == ',' {
			return i
		}
		// A number announces an address only when a space precedes it, so
		// the first token is never cut this way either.
		if i > 0 {
			if prev, _ := utf8.DecodeLastRuneInString(s[:i]); unicode.IsSpace(prev) && digitRun(s[i:]) >= 2 {
				return i
			}
		}
		i += w
	}
	return -1
}

// hasMarker reports whether s opens with an address marker that ends on a
// word boundary.
func hasMarker(s string) bool {
	for _, m := range addressMarkers {
		if !strings.HasPrefix(s, m) {
			continue
		}
		// The boundary the reference spells \b: it exists where a word
		// character meets a non-word one. A marker ending in a full stop
		// therefore needs a word character after it, which is why
		// "UL. KOMSOMOLSKAYA" is found by the number rather than by "UL.".
		last, _ := utf8.DecodeLastRuneInString(m)
		next, _ := utf8.DecodeRuneInString(s[len(m):])
		if isWord(last) != (len(s) > len(m) && isWord(next)) {
			return true
		}
	}
	return false
}

func digitRun(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsDigit(r) {
			break
		}
		n++
	}
	return n
}

// isWord is the reference's \w for text: a letter, a number, or an
// underscore. Go's regexp \w is ASCII, which would make "ДЖОНДОРИ"
// punctuation.
func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// StripAddress drops the address and keeps the name. The first token is never
// cut: "1SERVICE" is a name, not a house number.
func StripAddress(name string) string {
	if name == "" {
		return name
	}
	head := name
	if i := strings.Index(name, " "); i >= 0 {
		head = name[:i]
	}
	rest := name[len(head):]
	if at := findAddress(rest); at >= 0 {
		return strings.TrimSpace(head + rest[:at])
	}
	return strings.TrimSpace(name)
}

// Tier names the evidence a key rests on. It is not decoration: a caller
// deciding whether to auto-accept a proposal should treat a regulator-issued
// identifier and a name that survived normalisation differently.
type Tier string

const (
	TierTaxID   Tier = "tax_id"
	TierName    Tier = "name"
	TierAccount Tier = "account"
	TierNone    Tier = "none"
)

// quoteChars are dropped before the legal forms are stripped, so that
// «ООО "Ромашка"» and ООО Ромашка reach the same key.
const quoteChars = "\"'`"

// CounterpartyKey is the stable identity of a counterparty, and the tier that
// produced it.
//
// In CIS statements the tax number arrives in its own column -- УНП in
// Belarus, БИН/ИИН in Kazakhstan -- which is why it is the first tier rather
// than something parsed out of free text.
func CounterpartyKey(name, taxID, account string) (string, Tier) {
	// Eight digits is the shortest real identifier in the corpus; "0" and a
	// column of zeroes are how these exports spell "absent".
	if tid := keepDigits(taxID); tid != "" && tid != "0" && utf8.RuneCountInString(tid) >= 8 {
		return "tax:" + tid, TierTaxID
	}

	n := upper.String(quotes.Replace(norm.NFKC.String(StripAddress(name))))
	n = mapRunes(n, func(r rune) bool { return strings.ContainsRune(quoteChars, r) }, ' ')
	n = stripLegalForms(n)
	n = mapRunes(n, func(r rune) bool { return !isWord(r) && !unicode.IsSpace(r) }, ' ')
	n = strings.Join(strings.FieldsFunc(n, unicode.IsSpace), "")
	if n != "" {
		return "name:" + n, TierName
	}

	// Last resort. A bank reissues an account far more readily than a
	// counterparty changes its name, so a key built from one survives less.
	acc := mapRunes(upper.String(account), func(r rune) bool { return !isWord(r) }, -1)
	if acc == "" {
		return "", TierNone
	}
	return "acct:" + acc, TierAccount
}

func keepDigits(s string) string {
	return mapRunes(s, func(r rune) bool { return !unicode.IsDigit(r) }, -1)
}

// mapRunes replaces every rune matching with the given rune, or drops it when
// with is negative.
func mapRunes(s string, match func(rune) bool, with rune) string {
	return strings.Map(func(r rune) rune {
		if match(r) {
			return with
		}
		return r
	}, s)
}
