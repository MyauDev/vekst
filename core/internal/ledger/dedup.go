package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// fieldSeparator joins DedupHash's fields. A control character, not a comma
// or a pipe: none of the fields it separates is validated against ever
// containing one, so a description that happens to contain the separator a
// human would pick cannot make two different rows hash the same.
const fieldSeparator = "\x1f"

// ContentKey is design D5's input set, without the occurrence term: what a
// duplicate check must agree on before occurrence is even asked. Exported
// for core/internal/dedup, which counts repeats of it within one batch to
// assign each row's occurrence -- the one place outside this file that
// needs to agree on exactly this set of fields, which is why it reuses this
// rather than building its own.
func ContentKey(t Transaction) string {
	return strings.Join([]string{
		t.AccountID.String(),
		t.BookedOn.Format("2006-01-02"),
		strconv.FormatInt(t.Amount.MinorUnits, 10),
		t.Amount.CurrencyCode,
		t.DescriptionNorm,
		t.BankRef,
		t.DocumentRef,
		strconv.Itoa(int(t.PostingNo)),
	}, fieldSeparator)
}

// DedupHash is design D5's content hash: account_id, booked_on,
// amount_minor, currency, description_norm, bank_ref, document_ref and
// posting_no, plus the occurrence index of that exact content within its
// batch.
//
// The occurrence term is the difference between working and losing money.
// Two genuinely distinct payments -- coffee bought twice on the same day,
// same amount, same wording, no bank reference -- get occurrences 1 and 2,
// different hashes, and both are stored: the statement shows two lines
// because there were two payments. The same file imported twice reproduces
// the same occurrences and the same hashes, which is what lets
// core/internal/dedup's D3 recognise a re-import by comparing hashes --
// transactions_dedup_idx (org_id, dedup_hash) is not unique (corrected by
// add-dedup's task 0.4): recognising and skipping a match is application
// code's job, in core/internal/dedup, because the hash can still coincide
// for two unrelated real transactions across unrelated batches, which a
// hard database constraint could not tell apart from a real re-import.
//
// Callers are responsible for computing occurrence themselves -- the Nth
// time (1-based) this exact ContentKey has been seen so far while building
// one batch of transactions -- and for calling this once per row before
// Insert. Insert writes whatever DedupHash it is given; it does not compute
// one, so that the field set stays written down in exactly this one place.
func DedupHash(t Transaction, occurrence int) string {
	sum := sha256.Sum256([]byte(ContentKey(t) + fieldSeparator + strconv.Itoa(occurrence)))
	return hex.EncodeToString(sum[:])
}
