package classify_test

import (
	"context"
	"testing"
	"time"

	"github.com/MyauDev/vekst/core/classify"
	internalv1 "github.com/MyauDev/vekst/core/gen/vekstinternal/v1"
	"github.com/MyauDev/vekst/core/internal/money"
)

// Task 6.10. Everything a batch carries has to survive the translation in both
// directions, and money has to arrive as money: a currency code dropped on the
// way out turns a rule that matches over 1000 KZT into one that matches over
// 1000 of nothing.
func TestGRPCClientRoundTripsABatch(t *testing.T) {
	impl := &stubServer{
		version: "engine-1",
		proposals: []*internalv1.Proposal{{
			TransactionId:       "txn-1",
			CategoryCode:        "0403",
			EngineLayer:         "L0.5",
			Confidence:          0.99,
			Evidence:            "country:KZ",
			MatchedRulePriority: 51,
		}},
	}
	c := dial(t, serve(t, impl))

	req := classify.BatchRequest{
		RequestID:        "chunk-7",
		TaxonomyVersion:  "v1",
		RulesetVersion:   "v1",
		NormalizeVersion: "v1",
		Threshold:        0.8,
		Categories: []classify.Category{
			{Code: "0403", Name: "PAYROLL TAX", RequiresAllocation: true},
		},
		Rules: []classify.Rule{{
			Priority:     51,
			CategoryCode: "0403",
			Scope:        "country:KZ",
			SourceKind:   "bank",
			All: []classify.Condition{
				{Field: "regulated_code", Op: "eq", Value: "332"},
				{
					Field:       "amount",
					Op:          "gte",
					AmountValue: money.Money{CurrencyCode: "KZT", MinorUnits: 100000},
				},
			},
		}},
		Vendors: []classify.VendorMemory{
			{Key: "tax:120970530", CategoryCode: "0403", DisplayName: "ACME"},
		},
		Txns: []classify.Txn{{
			TransactionID:   "txn-1",
			SourceKind:      "bank",
			DescriptionNorm: "ZARABOTNAYA PLATA",
			CounterpartyKey: "tax:120970530",
			Direction:       "expense",
			Amount:          money.Money{CurrencyCode: "KZT", MinorUnits: 250000},
			RegulatedCode:   "332",
			AccountID:       "acc-1",
		}},
	}

	resp, err := c.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	sent := impl.gotBatch
	if sent.GetRequestId() != "chunk-7" || sent.GetNormalizeVersion() != "v1" {
		t.Errorf("request identity lost: %+v", sent)
	}
	if got := len(sent.GetRules()); got != 1 {
		t.Fatalf("rules sent = %d, want 1", got)
	}
	rule := sent.GetRules()[0]
	if rule.GetSourceKind() != "bank" {
		t.Errorf("rule source_kind = %q; a bank rule that loses it fires on ledger rows too",
			rule.GetSourceKind())
	}
	amount := rule.GetAll()[1].GetAmountValue()
	if amount.GetCurrencyCode() != "KZT" || amount.GetMinorUnits() != 100000 {
		t.Errorf("rule amount = %v, want 100000 KZT", amount)
	}
	txn := sent.GetTxns()[0]
	if txn.GetAmount().GetCurrencyCode() != "KZT" || txn.GetAmount().GetMinorUnits() != 250000 {
		t.Errorf("txn amount = %v, want 250000 KZT", txn.GetAmount())
	}
	if txn.GetRegulatedCode() != "332" || txn.GetCounterpartyKey() != "tax:120970530" {
		t.Errorf("txn fields lost: %+v", txn)
	}
	if sent.GetVendors()[0].GetKey() != "tax:120970530" {
		t.Errorf("vendor memory lost: %+v", sent.GetVendors())
	}
	if sent.GetCategories()[0].GetRequiresAllocation() != true {
		t.Error("requires_allocation lost: the engine would offer payroll as an ordinary leaf")
	}

	want := classify.Proposal{
		TransactionID:       "txn-1",
		CategoryCode:        "0403",
		EngineLayer:         "L0.5",
		Confidence:          0.99,
		Evidence:            "country:KZ",
		MatchedRulePriority: 51,
	}
	if len(resp.Proposals) != 1 || resp.Proposals[0] != want {
		t.Errorf("proposals = %+v, want [%+v]", resp.Proposals, want)
	}
	if resp.EngineVersion != "engine-1" || resp.RulesetVersion != "v1" {
		t.Errorf("response versions = %q/%q", resp.EngineVersion, resp.RulesetVersion)
	}
}

// A rule that does not match on an amount must not send a zero in an empty
// currency. "No amount" and "zero of a currency nobody named" are different
// claims, and only the first is true here.
func TestAnAbsentAmountStaysAbsent(t *testing.T) {
	impl := &stubServer{version: "engine-1"}
	c := dial(t, serve(t, impl))

	_, err := c.Classify(context.Background(), classify.BatchRequest{
		Rules: []classify.Rule{{
			Priority: 1,
			All:      []classify.Condition{{Field: "direction", Op: "eq", Value: "expense"}},
		}},
		Txns: []classify.Txn{{TransactionID: "t"}},
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	if got := impl.gotBatch.GetRules()[0].GetAll()[0].GetAmountValue(); got != nil {
		t.Errorf("amount_value = %v, want it left unset", got)
	}
	if got := impl.gotBatch.GetTxns()[0].GetAmount(); got != nil {
		t.Errorf("txn amount = %v, want it left unset", got)
	}
}

// ARCHITECTURE.md 3.5, for Classify as much as for Version: a classifier that
// is not answering is a retryable condition, and a worker must not be made to
// wait on one.
func TestClassifyRespectsDeadline(t *testing.T) {
	c := dial(t, serve(t, &stubServer{delay: time.Hour}))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := c.Classify(ctx, classify.BatchRequest{RequestID: "slow"}); err == nil {
		t.Error("Classify() ignored its deadline")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Classify() took %s; it must honour the caller's deadline", elapsed)
	}
}

func TestClassifyErrorsWhenServerIsDown(t *testing.T) {
	dead := dial(t, "127.0.0.1:1")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := dead.Classify(ctx, classify.BatchRequest{RequestID: "x"}); err == nil {
		t.Error("Classify() against a dead address returned no error")
	}
}

func TestUnavailableRefusesToClassify(t *testing.T) {
	var c classify.Classifier = classify.Unavailable{}

	resp, err := c.Classify(context.Background(), classify.BatchRequest{})
	if err == nil {
		t.Fatal("Unavailable.Classify() returned no error")
	}
	if len(resp.Proposals) != 0 {
		t.Errorf("Unavailable.Classify() returned %d proposals, want none", len(resp.Proposals))
	}
}

func dial(t *testing.T, addr string) *classify.GRPCClient {
	t.Helper()
	c, err := classify.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
