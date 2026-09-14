package ingest

import (
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildXLSX writes a workbook with rows shaped like a redacted Priorbank
// statement -- the same real (redacted) content already committed in
// core/testdata/priorbank-by, just in a different container. Numeric-looking
// values are written as text cells, comma-decimal and all, exactly as a bank
// export represents them: this test is about whether XLSX's own cell text
// round-trips through excelize correctly, not about Excel's numeric
// formatting, which needs a real XLSX bank export to get right and is
// blocked on the same private data task 6.1 is (see tasks.md).
func buildXLSX(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	sheet := f.GetSheetName(0)
	for r, row := range rows {
		for c, cell := range row {
			ref, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName: %v", err)
			}
			if err := f.SetCellStr(sheet, ref, cell); err != nil {
				t.Fatalf("SetCellStr(%s): %v", ref, err)
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer: %v", err)
	}
	return buf.Bytes()
}

func TestReadXLSXRowsRoundTripsRealRedactedContent(t *testing.T) {
	want := [][]string{
		{"Приорбанк Открытое акционерное общество, БИК PJCBBY2X", "27.05.2025 16:23:08"},
		{"Дата док.", "N док.", "Корреспондент.Название", "Номинал.Дебет", "Номинал.Кредит"},
		{"04.01.2023", "40", "ЯНТАРЬ ЛОГИСТИК", "0,00", "8 079,44"},
		{"05.01.2023", "40", "ЯНТАРЬ ЛОГИСТИК", "0,00", "5 117,44"},
	}

	got, err := ReadXLSXRows(buildXLSX(t, want))
	if err != nil {
		t.Fatalf("ReadXLSXRows: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for r := range want {
		if len(got[r]) < len(want[r]) {
			t.Fatalf("row %d: got %d cells, want at least %d: %v", r, len(got[r]), len(want[r]), got[r])
		}
		for c := range want[r] {
			if got[r][c] != want[r][c] {
				t.Errorf("row %d cell %d = %q, want %q", r, c, got[r][c], want[r][c])
			}
		}
	}
}

func TestReadXLSXRowsRejectsANonWorkbook(t *testing.T) {
	if _, err := ReadXLSXRows([]byte("date,amount\n2026-01-01,100\n")); err == nil {
		t.Fatal("ReadXLSXRows accepted plain CSV bytes")
	}
}

func TestIsXLSXDetectsTheZipMagicBytes(t *testing.T) {
	if !isXLSX(buildXLSX(t, [][]string{{"x"}})) {
		t.Error("isXLSX(real workbook) = false")
	}
	if isXLSX([]byte("date,amount\n2026-01-01,100\n")) {
		t.Error("isXLSX(csv) = true")
	}
}
