package ingest

import "testing"

func TestSniffContentType(t *testing.T) {
	tests := []struct {
		name string
		head []byte
		want string
	}{
		{"xlsx magic bytes", []byte("PK\x03\x04rest of the zip"), contentTypeXLSX},
		{"csv text", []byte("date,amount,description\n2026-01-01,100,coffee\n"), contentTypeText},
		{"nul byte marks binary", []byte("date,amount\x00,description"), contentTypeOctetStream},
		{"empty file", []byte{}, contentTypeText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sniffContentType(tt.head); got != tt.want {
				t.Errorf("sniffContentType(%q) = %q, want %q", tt.head, got, tt.want)
			}
		})
	}
}

// The declared type must never change the answer -- design D2's point in
// full: nothing the client says about its own upload is a fact.
func TestSniffContentTypeIgnoresNothingButTheBytes(t *testing.T) {
	csv := []byte("a,b,c\n1,2,3\n")
	if got := sniffContentType(csv); got != contentTypeText {
		t.Errorf("sniffContentType ignored nothing to consult -- got %q", got)
	}
}
