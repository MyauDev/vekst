// Package ingest owns upload, parsing, validation and persistence of source
// documents. Track A.
//
// It reads a bank's own export format and produces canonical rows.
//
// Everything here is a pure function over bytes: no database handle, no clock,
// no network. A parser is the one place in this system that meets a file
// nobody in this team wrote, so it fails loudly and it says which line failed
// -- by its number in the original file, never by an index into whatever
// survived parsing. Somebody has to find that row in Excel.
//
// Validation is not here. This package answers "what does the file say"; change
// 2.3 answers "is what it says complete and correct".
package ingest

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Charset names an encoding this package can decode.
type Charset string

const (
	CharsetUTF8       Charset = "utf-8"
	CharsetWindows125 Charset = "windows-1251"
	CharsetCP866      Charset = "cp866"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// DetectCharset guesses the encoding of a CIS bank export.
//
// The order matters. A byte sequence that is valid UTF-8 is almost never
// accidentally valid UTF-8 -- Cyrillic in windows-1251 is single bytes in the
// 0xC0-0xFF range, which fail UTF-8's continuation rules almost immediately --
// so "valid UTF-8" is a reliable positive. Everything else is decided by which
// single-byte page produces more Cyrillic letters and fewer box-drawing
// characters, because CP866 and windows-1251 disagree about exactly that range.
func DetectCharset(raw []byte) Charset {
	if bytes.HasPrefix(raw, utf8BOM) || utf8.Valid(raw) {
		return CharsetUTF8
	}
	if scoreCyrillic(charmap.Windows1251, raw) >= scoreCyrillic(charmap.CodePage866, raw) {
		return CharsetWindows125
	}
	return CharsetCP866
}

// Decode converts raw bytes to a UTF-8 string, stripping a BOM if present.
//
// A U+FFFD in the result means the charset guess was wrong. This function does
// not treat that as an error -- change 2.3 does, and it rejects the batch,
// because a replacement character is silent data corruption rather than a
// cosmetic problem.
func Decode(raw []byte, cs Charset) (string, error) {
	switch cs {
	case CharsetUTF8:
		return string(bytes.TrimPrefix(raw, utf8BOM)), nil
	case CharsetWindows125:
		return decodeWith(charmap.Windows1251, raw)
	case CharsetCP866:
		return decodeWith(charmap.CodePage866, raw)
	default:
		return "", fmt.Errorf("ingest: unknown charset %q", cs)
	}
}

func decodeWith(cm *charmap.Charmap, raw []byte) (string, error) {
	out, err := cm.NewDecoder().Bytes(raw)
	if err != nil {
		return "", fmt.Errorf("ingest: decoding as %s: %w", cm, err)
	}
	return string(out), nil
}

// ContainsReplacementChar reports whether decoding produced U+FFFD anywhere.
func ContainsReplacementChar(s string) bool {
	return bytes.ContainsRune([]byte(s), utf8.RuneError)
}

func scoreCyrillic(cm *charmap.Charmap, raw []byte) int {
	// A sample is enough: these files are homogeneous, and a statement's
	// first kilobytes are its header, which is always prose.
	if len(raw) > 8192 {
		raw = raw[:8192]
	}
	decoded, err := cm.NewDecoder().Bytes(raw)
	if err != nil {
		return 0
	}
	score := 0
	for _, r := range string(decoded) {
		switch {
		case (r >= 'А' && r <= 'я') || r == 'ё' || r == 'Ё':
			score++
		case r >= 0x2500 && r <= 0x25FF: // box drawing: the wrong page's tell
			score -= 2
		}
	}
	return score
}
