package ingest

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// isXLSX reports whether raw looks like an OOXML spreadsheet: a ZIP archive,
// which is what XLSX is, identified the same way blob's content-type
// sniffing does -- the local file header every ZIP-based format starts with.
// It is not certain: a ZIP that happens to start this way but is not an
// XLSX still opens successfully here and fails inside ReadXLSXRows instead,
// which is where the real answer -- does this parse as a workbook at all --
// actually lives.
func isXLSX(raw []byte) bool {
	return bytes.HasPrefix(raw, []byte("PK\x03\x04"))
}

// ReadXLSXRows opens an XLSX workbook and returns every row of its first
// sheet as string cells, in file order.
//
// Unlike a bank's own CSV export, there is no charset to detect: OOXML
// strings are UTF-8 by the format's own specification (ECMA-376), so this
// needs none of decode.go's scoring or DetectCharset. A numeric or date cell
// comes back as the text Excel would display for it, via excelize's own
// number-format rendering -- DecimalFromLocale and money.Parse still own
// turning that text into minor units exactly the way they own a CSV
// export's text, so no float appears on this path either.
func ReadXLSXRows(raw []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("ingest: opening xlsx: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("ingest: xlsx has no sheets")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("ingest: reading sheet %q: %w", sheets[0], err)
	}
	return rows, nil
}
