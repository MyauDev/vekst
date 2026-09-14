// Package ledger is the typed seam onto the canonical ledger (change 2.5,
// migration 007): transactions and their classifications, as Go values
// rather than as generated rows. A caller builds a Transaction, calls
// Insert inside its own db.InTx, and never sees a pgtype or a gendb struct.
package ledger

import (
	"time"

	"github.com/google/uuid"

	"github.com/MyauDev/vekst/core/internal/money"
)

// SourceKind values. The single most load-bearing column in the schema
// (migration 007's own comment): a report line is computed from one of
// these and never from both.
const (
	SourceKindLedger = "ledger"
	SourceKindBank   = "bank"
)

// Direction values.
const (
	DirectionIncome  = "income"
	DirectionExpense = "expense"
)

// EngineLayer values a classification may carry.
const (
	EngineLayerL0    = "L0"
	EngineLayerL05   = "L0.5"
	EngineLayerL1    = "L1"
	EngineLayerL2    = "L2"
	EngineLayerHuman = "human"
)

// FXConversion is the conversion recorded on a Transaction, never
// recomputed (design D2). Its four columns in migration 007 are
// all-or-nothing; nesting them behind one pointer makes the partial state
// unrepresentable in Go, on top of the database's own CHECK.
type FXConversion struct {
	// The exact decimal text of numeric(20,10) -- never a float, because a
	// rate multiplies money and "0.1 + 0.2" is as untrue here as anywhere.
	Rate   string
	RateOn time.Time
	// The converted amount, already in Base.CurrencyCode.
	Base money.Money
}

// Transaction is one row of the grain ARCHITECTURE.md 5.0 describes: a bank
// payment (DocumentRef "", PostingNo 0) or one posting of a ledger document
// (DocumentRef set, PostingNo > 0, siblings sharing the same DocumentRef).
type Transaction struct {
	ID       uuid.UUID
	EntityID uuid.UUID
	// The account the amount moved through.
	AccountID uuid.UUID
	// The import that produced this row.
	BatchID uuid.UUID

	// ledger | bank. Denormalised from the batch on purpose (design D1);
	// migration 007's own trigger holds this equal to the batch's.
	SourceKind string

	// "" means NULL: a bank payment, alone on its own line. Non-empty means
	// this row is one posting of the ledger document it names, and
	// PostingNo is then > 0.
	DocumentRef string
	PostingNo   int32

	BookedOn time.Time
	// The zero time.Time means NULL: not every source states a value date.
	ValueOn time.Time

	// income | expense.
	Direction string

	// What the bank or the ledger said.
	Amount money.Money
	// nil means this row is already in the organisation's base currency;
	// non-nil means it was converted, and how (design D2).
	FX *FXConversion

	CounterpartyRaw string
	CounterpartyKey string
	DescriptionRaw  string
	DescriptionNorm string

	// What produced DescriptionNorm and CounterpartyKey (design D3). Read
	// back from the row and sent to the classifier, never taken from
	// current configuration.
	NormalizeVersion string

	// КНП, Typ operacji, a 1C account code. "" when the source carried none.
	RegulatedCode string

	BankRef string

	// The content hash design D5 describes, occurrence term included.
	// DedupHash computes it; Insert only ever writes what it is given.
	DedupHash string

	CreatedAt time.Time
}

// Classification is one answer, live or superseded. A correction is a new
// row plus a pointer at the old one (design D4) -- there is no update to a
// category once written.
type Classification struct {
	ID            uuid.UUID
	TransactionID uuid.UUID
	// Not a foreign key at the database level: categories is shared and
	// tenant at once (see migration 007's own comment). Still a plain UUID
	// here -- the constraint trigger is what enforces "visible, a leaf, not
	// computed", not this type.
	CategoryID uuid.UUID

	// L0 | L0.5 | L1 | L2 | human.
	EngineLayer string
	// A probability, not money: float64 is the right type, the same way
	// vekst.internal.v1's own confidence field is a double.
	Confidence float64
	Evidence   string

	// The three strings a report pins, plus the one recording how the text
	// they matched on was normalised. All four travel together because a
	// report is reproducible only if every input to it is recorded beside
	// the output.
	TaxonomyVersion  string
	RulesetVersion   string
	EngineVersion    string
	NormalizeVersion string

	// The zero UUID means a machine decided; migration 007's own CHECK
	// requires a human decision to name one.
	DecidedBy uuid.UUID
	DecidedAt time.Time
	// The zero UUID means this classification is live.
	SupersededBy uuid.UUID
}
