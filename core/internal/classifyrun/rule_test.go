package classifyrun

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/MyauDev/vekst/core/classify"
	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
)

// Reading a stored rule, without a database.
//
// The asymmetry these assert is the whole point of the function: an unreadable
// rule fails the run rather than being skipped. Skipping is worse than failing
// here, and it is worth being precise about why -- a rule that loses a condition
// does not stop matching, it starts matching everything its remaining
// conditions allow. "Description contains X and direction is expense" that
// quietly becomes "direction is expense" puts every payment a business makes on
// one line, and nothing anywhere says so.

func ruleRow(matcher string) gendb.EffectiveRulesRow {
	return gendb.EffectiveRulesRow{
		ID:           pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		Priority:     7,
		Scope:        "country:BY",
		CategoryCode: "0404",
		Matcher:      []byte(matcher),
	}
}

func TestToRuleReadsASeededMatcher(t *testing.T) {
	// The shape migration 006 actually seeds, copied from one of its 71 rows.
	rule, err := toRule(ruleRow(`{"source_kind": "bank", "normalize_version": "v1",
		"all": [{"field": "description", "op": "contains_all", "value": "ПОДОХОДНЫЙ НАЛОГ"},
		        {"field": "direction", "op": "eq", "value": "expense"}]}`))
	if err != nil {
		t.Fatalf("toRule: %v", err)
	}

	if rule.Priority != 7 || rule.CategoryCode != "0404" || rule.Scope != "country:BY" {
		t.Errorf("rule = %+v", rule)
	}
	if rule.SourceKind != "bank" {
		t.Errorf("source_kind = %q; a rule that did not say is tried on both, and one that "+
			"did must not be", rule.SourceKind)
	}
	if len(rule.All) != 2 {
		t.Fatalf("%d conditions, want 2", len(rule.All))
	}
	if rule.All[0].Field != "description" || rule.All[0].Op != "contains_all" {
		t.Errorf("first condition = %+v", rule.All[0])
	}
	if rule.All[1].Value != "expense" {
		t.Errorf("second condition lost its comparand: %+v", rule.All[1])
	}
}

// Amount conditions, which no seeded rule uses and a customer's own rule will.
// Money is minor units plus a code here as everywhere else: a float in a rule
// would compare a customer's threshold against their transactions
// approximately.
func TestToRuleReadsAnAmountCondition(t *testing.T) {
	rule, err := toRule(ruleRow(`{"all": [{"field": "amount", "op": "gte",
		"amount": {"currency_code": "BYN", "minor_units": 100000}}]}`))
	if err != nil {
		t.Fatalf("toRule: %v", err)
	}
	got := rule.All[0].AmountValue
	if got.CurrencyCode != "BYN" || got.MinorUnits != 100000 {
		t.Errorf("amount = %+v, want 100000 BYN", got)
	}
}

func TestToRuleRefusesWhatItCannotRead(t *testing.T) {
	for name, matcher := range map[string]string{
		"not JSON at all":              `not json`,
		"no conditions":                `{"all": []}`,
		"no `all` key":                 `{"source_kind": "bank"}`,
		"a condition with no field":    `{"all": [{"op": "eq", "value": "expense"}]}`,
		"a condition with no operator": `{"all": [{"field": "direction", "value": "expense"}]}`,
		// An amount in a currency the ISO-4217 table does not know is not an
		// amount, and defaulting its exponent would be a guess about somebody's
		// money.
		"an unknown currency": `{"all": [{"field": "amount", "op": "gte",
			"amount": {"currency_code": "XXZ", "minor_units": 1}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := toRule(ruleRow(matcher)); err == nil {
				t.Error("accepted. A rule this process cannot read must fail the run: " +
					"skipping it does not stop it matching, it widens what it matches")
			}
		})
	}
}

// The failure a run records travels as a code, and the cause it wraps stays
// reachable for a log without reaching a screen.
func TestAFatalErrorCarriesItsCodeAndItsCause(t *testing.T) {
	cause := errors.New("proposal names category \"no-such-code\"")
	err := error(&fatalError{code: FailureUnknownCategory, err: cause})

	if !strings.HasPrefix(err.Error(), FailureUnknownCategory) {
		t.Errorf("Error() = %q, want it to lead with the code", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Error("the cause is not reachable through the wrapper")
	}

	var fatal *fatalError
	if !errors.As(err, &fatal) || fatal.code != FailureUnknownCategory {
		t.Error("a fatal error did not survive errors.As")
	}
}

// isUniqueViolation tests the SQLSTATE and the constraint name, not the
// message. A check on wording breaks on a Postgres upgrade and, worse, can
// match a different constraint that happens to be worded alike -- which here
// would mean a second run being mistaken for a duplicate enqueue and silently
// doing nothing.
func TestIsUniqueViolationIsNarrow(t *testing.T) {
	if isUniqueViolation(errors.New("duplicate key value violates unique constraint"), "anything") {
		t.Error("a plain error matching on wording was read as a constraint violation")
	}
	if isUniqueViolation(nil, "anything") {
		t.Error("nil was read as a violation")
	}
}

// The job's kind is what River looks a worker up by, and what the enqueue in
// another package inserts. It is a string in two places and this is the one
// that is allowed to change it.
func TestTheJobRegistersUnderItsKind(t *testing.T) {
	if got := (ClassifyBatchArgs{}).Kind(); got != "classify_batch" {
		t.Errorf("Kind() = %q; core/internal/ingest's own test reads river_job by this "+
			"exact string", got)
	}

	// Register puts a worker in the bundle under that kind. A registrar that
	// added nothing would make every enqueue fail with UnknownJobKindError --
	// and because the enqueue happens inside the transaction that lands a batch
	// on `imported`, that failure lands on the *import*, not on the
	// classification.
	workers := river.NewWorkers()
	w := NewWorkers(nil, classify.Unavailable{}, Versions{Taxonomy: "v1", Ruleset: "v1"})
	if periodic := w.Register(workers); len(periodic) != 0 {
		t.Errorf("%d periodic jobs; every job here is enqueued for a named batch, and a "+
			"sweep would have no tenant context of its own to run under", len(periodic))
	}

	// That it registered anything at all is observable exactly one way: River
	// panics when two workers claim one kind, so a second Register on the same
	// bundle has to. A registrar that quietly added nothing would pass every
	// other assertion here and fail every enqueue in production.
	defer func() {
		if recover() == nil {
			t.Error("registering twice did not collide, so the first Register added no " +
				"worker under this kind")
		}
	}()
	w.Register(workers)
}

// The tenant never comes from ambient state.
func TestTheWorkerTakesItsTenantFromItsArguments(t *testing.T) {
	if _, err := db.OrgIDFromJobArgs(ClassifyBatchArgs{}.TenantJobArgs); err == nil {
		t.Error("a job with no organisation produced a tenant identifier; River's tables " +
			"carry no org_id and no policy, so a payload is untrusted input for tenancy")
	}
}
