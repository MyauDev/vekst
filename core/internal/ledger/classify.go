package ledger

import "github.com/MyauDev/vekst/core/classify"

// ToClassifyBatch turns a page of Transaction into the Txns half of a
// classify.BatchRequest -- the part a Transaction alone can answer.
// Categories, Rules, Vendors and the three pinned versions come from a
// different read; a caller assembles the rest, and the worker a later
// change writes has nothing to invent about how a Transaction becomes one
// of these (design D7: the id crossing to the classifier is the database's
// own, rendered as text).
func ToClassifyBatch(txns []Transaction) []classify.Txn {
	out := make([]classify.Txn, len(txns))
	for i, t := range txns {
		out[i] = classify.Txn{
			TransactionID:   t.ID.String(),
			SourceKind:      t.SourceKind,
			DescriptionNorm: t.DescriptionNorm,
			CounterpartyKey: t.CounterpartyKey,
			Direction:       t.Direction,
			Amount:          t.Amount,
			RegulatedCode:   t.RegulatedCode,
			AccountID:       t.AccountID.String(),
		}
	}
	return out
}
