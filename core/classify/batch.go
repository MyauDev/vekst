package classify

import "github.com/MyauDev/vekst/core/internal/money"

// The types a batch is made of.
//
// They are core's own, not the generated protobuf ones, for the same reason
// VersionInfo is: a caller of this package should not have to import the wire
// contract to ask a question, and money crossing this boundary is money.Money
// rather than a pair of fields a caller could build inconsistently. GRPCClient
// translates, in one place, and that translation is the only code that knows
// the request is protobuf at all.

// Category is a leaf a classification may target. Sections and computed lines
// never appear here -- the query that fills it excludes them, because an
// amount landing on one would be counted twice.
type Category struct {
	Code               string
	Name               string
	RequiresAllocation bool
}

// Condition is one test in a rule's AND.
type Condition struct {
	// description | counterparty_key | regulated_code | direction | amount |
	// account
	Field string
	// contains_all | eq | gte | lte
	Op string
	// The comparand for every op but the amount ones.
	Value string
	// The comparand for gte and lte. Zero when Field is not "amount".
	AmountValue money.Money
}

// Rule is one L0.5 or L1 rule, already in the order the engine must try it.
type Rule struct {
	Priority     int32
	CategoryCode string
	// 'country:BY' | 'bank:priorbank' | 'org'. Returned as a proposal's
	// evidence, so "why this category" answers with how a country words its
	// payments rather than with a rule id.
	Scope string
	// ledger | bank. Empty means the rule did not say, and it is tried on
	// both.
	SourceKind string
	All        []Condition
}

// VendorMemory is one decision this organisation already made.
type VendorMemory struct {
	Key          string
	CategoryCode string
	DisplayName  string
}

// Txn is a transaction as the engine sees it: already normalised, with no
// amount history, no document and no counterparty record. What is not here
// cannot influence the answer, which is what makes a batch reproducible.
type Txn struct {
	TransactionID string
	// ledger | bank
	SourceKind      string
	DescriptionNorm string
	CounterpartyKey string
	// income | expense
	Direction     string
	Amount        money.Money
	RegulatedCode string
	AccountID     string
}

// BatchRequest is everything the engine reasons with. There is no fourth
// source of truth it could consult: it holds no database credentials, by
// design (ARCHITECTURE.md A-4).
type BatchRequest struct {
	// Idempotency key for one chunk, so that a retried job does not produce a
	// second set of proposals for rows it already has.
	RequestID string

	// The three strings a report pins, plus the one that says how the text in
	// this request was normalised.
	TaxonomyVersion  string
	RulesetVersion   string
	NormalizeVersion string

	Categories []Category
	Rules      []Rule
	Vendors    []VendorMemory
	Txns       []Txn

	// Below this a proposal is not returned. Zero means the engine's own
	// default, which is stricter than "accept everything" -- proto3 cannot
	// tell an unset double from a deliberate 0.0.
	Threshold float64
}

// Proposal is one answer. A transaction absent from a response is one no layer
// answered: a result, not an omission, and it goes to review.
type Proposal struct {
	TransactionID string
	CategoryCode  string
	// L0 | L0.5 | L1
	EngineLayer         string
	Confidence          float64
	Evidence            string
	MatchedRulePriority int32
}

// BatchResponse carries the engine build and the ruleset it answered under,
// both of which are stored on the classification rows this produces.
type BatchResponse struct {
	EngineVersion  string
	RulesetVersion string
	Proposals      []Proposal
}
