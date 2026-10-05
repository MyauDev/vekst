package migrate

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/MyauDev/vekst/core/internal/normalize"
)

// Migration 00019: the Belarusian revenue rules, and the ruleset version that
// carries them.
//
// The bug these answer is not a rule that classified something wrongly. It is
// the absence of any rule at all: of migration 006's 71 template rules, four
// reach `0101 SOFTWARE DEVELOPMENT` and all four are `country:KZ`. A Priorbank
// statement could not produce revenue, so NET SALES printed 0.00 for twelve
// consecutive months while 459,018.76 BYN of software payments sat in the
// unclassified bucket.

const rulesetV2 = "v2"

// The wordings these were measured against, copied from the statement rather
// than invented. Four counterparties, one service, four spellings -- which is
// the argument for stems over phrases, and it only holds if the stems really
// do occur in the text a bank writes.
var byRevenueWordings = []string{
	"ОПЛАТА ЗА СОПРОВОЖДЕНИЕ И РАЗРАБОТКУ ПО ДОГОВОРУ ДОГОВОР BI-23-03BYN ОТ  20.07.2023",
	"ЗА РАЗРАБОТКУ ПРОГРАММНОГО ОБЕСПЕЧЕНИЯ СОГЛАСНО АКТА N13 ОТ 29.03.24Г.",
	"ОПЛАТА ЗА АНАЛИЗ, ПРОЕКТИРОВАНИЕ И РАЗРАБОТКУ ПРОГРАМНОГО ОБЕСПЕЧЕНИЯ ПО ДОГОВОРУ",
	"ОПЛАТА (5 250 ДОЛ.ПО К.НБРБ 15.02.24) СОГЛ. ДОГОВОРА НА РАЗРАБОТКУ 1 ОТ 25.09.2023Г.",
	"ОПЛАТА ПО СЧЕТУ 1 ОТ 29 ЯНВАРЯ 2024 ГОДА ЗА ДОРАБОТКУ ПО (САЙТА)",
	"ПРЕДОПЛАТА ЗА ИНФОРМАЦИОННУЮ ПОДДЕРЖКУ,  ОБСЛУЖИВАНИЕ И МОНИТОРИНГ САЙТА ПО ДОГОВОРУ",
	"ИНФОРМ.ПОДДЕРЖКА, ОБСЛУЖИВАНИЕ И МОНИТОРИНГ САЙТА СФ 2 ОТ 02 ИЮЛЯ 2024",
	"ОПЛАТА ПО АКТУ 00000038 ОТ 31 ИЮЛЯ 2024 ГОДА ЗА ПОДДЕРЖКУ, ОБСЛУЖИВАНИЕ И МОНИТОРИНГ САЙТА",
}

// A new ruleset version is a whole ruleset, not a delta: EffectiveRules
// selects one `ruleset_version` and takes every rule under it. A v2 holding
// only the three new rules would be a deployment that classifies nothing else.
func TestRulesetV2CarriesV1ForwardAndAddsThree(t *testing.T) {
	f := newTenantFixture(t, "vekst_ruleset_v2_test")

	var v1, v2 int
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		if err := tx.QueryRow(`
			SELECT count(*) FROM classification_rules
			 WHERE taxonomy_version = $1 AND ruleset_version = $2 AND org_id IS NULL`,
			taxonomyV, rulesetV).Scan(&v1); err != nil {
			return err
		}
		return tx.QueryRow(`
			SELECT count(*) FROM classification_rules
			 WHERE taxonomy_version = $1 AND ruleset_version = $2 AND org_id IS NULL`,
			taxonomyV, rulesetV2).Scan(&v2)
	}); err != nil {
		t.Fatalf("counting rulesets: %v", err)
	}

	if v1 != seededRules {
		t.Errorf("ruleset v1 holds %d template rules, want %d -- v1 is what every "+
			"classification written before 00019 pins, and it has to keep meaning "+
			"what it meant", v1, seededRules)
	}
	if v2 != seededRules+3 {
		t.Errorf("ruleset v2 holds %d template rules, want %d", v2, seededRules+3)
	}
}

// The three rules, read back from the table rather than from the migration
// that wrote them.
func TestTheBYRevenueRulesReachTheRevenueLine(t *testing.T) {
	f := newTenantFixture(t, "vekst_by_revenue_test")

	type rule struct {
		priority int
		scope    string
		code     string
		matcher  string
	}
	var rules []rule
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT r.priority, r.scope, c.code, r.matcher::text
			  FROM classification_rules r
			  JOIN categories c ON c.id = r.category_id
			 WHERE r.taxonomy_version = $1 AND r.ruleset_version = $2
			   AND r.org_id IS NULL AND r.priority > $3
			 ORDER BY r.priority`, taxonomyV, rulesetV2, seededRules)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r rule
			if err := rows.Scan(&r.priority, &r.scope, &r.code, &r.matcher); err != nil {
				return err
			}
			rules = append(rules, r)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the new rules: %v", err)
	}

	if len(rules) != 3 {
		t.Fatalf("%d rules above priority %d, want 3", len(rules), seededRules)
	}

	for _, r := range rules {
		// `01 NET SALES` is not a leaf and nothing may be classified onto it
		// directly; `0101` is the only leaf beneath it, and reaching the
		// revenue line at all means reaching that one.
		if r.code != "0101" {
			t.Errorf("priority %d points at %q, not the revenue leaf 0101", r.priority, r.code)
		}
		if r.scope != "country:BY" {
			t.Errorf("priority %d is scoped %q; these were measured on Belarusian "+
				"wordings and say nothing about any other country's", r.priority, r.scope)
		}
		// The condition that keeps this company's own payments *for*
		// development off its revenue line. The statement these were measured
		// on has seven such rows, 13,766.06 of them.
		if !strings.Contains(r.matcher, `"value": "income"`) {
			t.Errorf("priority %d does not require direction=income: %s", r.priority, r.matcher)
		}
	}
}

// The stems have to occur in what a bank actually writes, after the same
// normalisation the engine matches against. This is the assertion that would
// have failed had the stems been chosen from memory rather than from a file:
// `contains_all` is a substring test over `description_norm`, so a stem that
// survives normalisation differently from the text around it matches nothing
// and says nothing about why.
func TestEveryBYRevenueWordingIsReachedByAStemOnTheRule(t *testing.T) {
	f := newTenantFixture(t, "vekst_by_revenue_stems_test")

	var stems []string
	if err := inTenantTx(t, f.app, orgA, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT cond ->> 'value'
			  FROM classification_rules r,
			       LATERAL jsonb_array_elements(r.matcher -> 'all') AS cond
			 WHERE r.taxonomy_version = $1 AND r.ruleset_version = $2
			   AND r.org_id IS NULL AND r.priority > $3
			   AND cond ->> 'field' = 'description'`, taxonomyV, rulesetV2, seededRules)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return err
			}
			stems = append(stems, s)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the stems: %v", err)
	}

	if len(stems) == 0 {
		t.Fatal("no description conditions on the new rules")
	}

	for _, wording := range byRevenueWordings {
		norm := normalize.Description(wording)
		matched := false
		for _, stem := range stems {
			// The engine splits on ';' and requires every part; none of these
			// three has one, and reproducing the split here would be testing
			// the test.
			if strings.Contains(norm, normalize.Description(stem)) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("no stem in %v reaches %q\n  normalised: %q", stems, wording, norm)
		}
	}
}

// And the other direction: the outgoing rows carrying the same words. The
// stems match them -- that is the point of the direction condition, not an
// accident it survives -- so this asserts the words really are ambiguous and
// that the rule is not relying on them not being.
func TestTheSameWordsAppearOnMoneyGoingOut(t *testing.T) {
	outgoing := "ОПЛАТА ЗА РАЗРАБОТКУ ПРОГРАММНОГО ОБЕСПЕЧЕНИЯ СОГЛАСНО ДОГОВОРУ"
	if !strings.Contains(normalize.Description(outgoing), normalize.Description("РАЗРАБОТК")) {
		t.Fatal("the fixture no longer demonstrates the ambiguity it exists to demonstrate")
	}
	// Nothing else is asserted here: what separates this row from revenue is
	// `direction`, which lives on the rule and is checked above. This test
	// exists so that a later reader tempted to drop that condition as
	// redundant can see, in one place, that it is not.
}
