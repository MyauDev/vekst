package ingest

import "bytes"

// sniffWindow bounds how much of the object's leading bytes decide its
// content type. 8 KiB is far past any bank's preamble and cheap to hold in
// memory regardless of the object's real size.
const sniffWindow = 8192

// contentTypeXLSX is what an XLSX's own magic bytes sniff to. XLSX is a ZIP
// container, and "PK\x03\x04" is the local file header every ZIP-based format
// starts with -- there is no narrower signature to look for without actually
// opening the archive, which is 2.2's job, not this one's.
const contentTypeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// contentTypeOctetStream marks a leading sample that contains a NUL byte --
// something no plain-text ledger or bank export, in any encoding this
// product reads, ever contains. It is a fact about the bytes, not a
// validation verdict: this change measures, it does not judge, so a batch
// sniffed this way still becomes 'uploaded' like any other. A parser
// declaring what it can read (core/internal/ingest's registry) is what
// eventually refuses it, at 2.2, which is a validation decision and belongs
// there.
const contentTypeOctetStream = "application/octet-stream"

const contentTypeText = "text/plain"

// sniffContentType inspects the bytes core actually read, never the type the
// client declared (design D2) -- declared_type is a signing condition, not a
// fact, and is never consulted here.
func sniffContentType(head []byte) string {
	if bytes.HasPrefix(head, []byte("PK\x03\x04")) {
		return contentTypeXLSX
	}
	n := len(head)
	if n > sniffWindow {
		n = sniffWindow
	}
	if bytes.IndexByte(head[:n], 0) >= 0 {
		return contentTypeOctetStream
	}
	return contentTypeText
}
