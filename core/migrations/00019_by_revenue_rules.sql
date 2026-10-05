-- Belarusian revenue rules, and the ruleset version that carries them.
--
-- Found by importing a real Priorbank statement: twelve months, 739 rows, and
-- NET SALES printed 0.00 in every one of them. Not a classification that went
-- wrong -- a classification that could not happen. Of the 71 rules migration
-- 006 seeds, exactly four reach `0101 SOFTWARE DEVELOPMENT`, the only leaf
-- under `01 NET SALES`, and all four are `country:KZ` matching Kazakh
-- payment-purpose codes 851, 857, 858 and 859. Belarus has 29 `country:BY`
-- rules and 12 `bank:priorbank` rules between them, and not one names revenue.
-- Poland has nine and the same hole.
--
-- So every Belarusian import produces a report whose first line is structurally
-- zero, with 459,018.76 BYN of software-development payments sitting in the
-- unclassified bucket, under a bottom line of 4.17M "profit" made entirely of
-- currency sales booked to Financial result. The three rules below are the
-- first half of that; the currency-sale half is not this migration's.
--
-- Measured before they were written, against that statement:
--
--   РАЗРАБОТК     35 rows   "ЗА РАЗРАБОТКУ ПРОГРАММНОГО ОБЕСПЕЧЕНИЯ",
--                           "ОПЛАТА ЗА СОПРОВОЖДЕНИЕ И РАЗРАБОТКУ",
--                           "СОГЛ. ДОГОВОРА НА РАЗРАБОТКУ",
--                           "ЗА АНАЛИЗ, ПРОЕКТИРОВАНИЕ И РАЗРАБОТКУ"
--   ПОДДЕРЖК       7 rows   "ЗА ИНФОРМАЦИОННУЮ ПОДДЕРЖКУ", "ИНФОРМ.ПОДДЕРЖКА",
--                           "ЗА ПОДДЕРЖКУ, ОБСЛУЖИВАНИЕ И МОНИТОРИНГ САЙТА"
--   ДОРАБОТК       1 row    "ЗА ДОРАБОТКУ ПО (САЙТА)" -- "РАЗРАБОТК" does not
--                           contain it, which is the whole reason it is here
--
-- 43 of 43, 457,025.61 of the 459,018.76; the remaining 1,993.15 is two refund
-- rows that rules 14 and 18 already answer. Stems rather than whole phrases,
-- because four counterparties word the same service four ways and the next
-- customer will word it a fifth.
--
-- `direction = income` is not decoration. The same statement has seven
-- *outgoing* rows carrying these words -- this company pays for development
-- too, 13,766.06 of it -- and without the direction condition they would land
-- on the revenue line as negative revenue. A rule that loses a condition does
-- not stop matching; it starts matching everything its remaining conditions
-- allow.
--
-- ---------------------------------------------------------------------------
-- Why a new ruleset version rather than three more rows in v1
-- ---------------------------------------------------------------------------
--
-- CLAUDE.md: "a report pins taxonomy_version + ruleset_version +
-- engine_version. Those three strings are what make a March report reproduce
-- in June, and an accountant will ask." Appending to v1 would make "v1" mean
-- one thing before this migration and another after it, which is the single
-- property those strings exist to have. Every classification already written
-- stays v1 and keeps meaning exactly what it meant.
--
-- EffectiveRules selects one `ruleset_version`, so v2 has to be the whole set
-- rather than the delta: v1's 71 rules are copied forward unchanged, then the
-- three new ones are appended at 72-74. Priorities are preserved, so the new
-- rules sort last. That is safe here and would not have been in general: no
-- existing rule matches an incoming row carrying these stems, BY's income
-- rules being the two refunds, the currency sale, interest on balances and
-- revaluation.
--
-- ---------------------------------------------------------------------------
-- Why the RLS window
-- ---------------------------------------------------------------------------
--
-- Two separate reasons, and neither is optional.
--
-- `classification_rules_write` is `org_id = app_current_org()`, and a template
-- row has `org_id IS NULL` -- so the policy refuses it under every tenant
-- context that could be set, including the nil one. Migration 006 inserted its
-- 71 rows *before* enabling RLS and never met this; there is no such ordering
-- available a migration later. NO FORCE for the duration is what the owner has
-- instead, and it is the same window 015 and 017 open for the same reason.
--
-- The nil-UUID tenant context is still needed on top of it, because
-- `rules_category_is_visible` fires on every inserted row and reads
-- `categories` -- forced by migration 005, and its policy calls
-- `app_current_org()`, which raises 42704 when nothing set it. 006 documents
-- that pair at length; this is the same pair.

-- +goose Up

SET LOCAL app.org_id = '00000000-0000-0000-0000-000000000000';

ALTER TABLE classification_rules NO FORCE ROW LEVEL SECURITY;

-- Nothing writes this table at runtime today: `EffectiveRules` is the only
-- query against it and it only reads, so v1 is 71 template rows and nothing
-- else. Were that to stop being true, the copy below would drop a customer's
-- own rules from v2 in silence and their corrections would come back as
-- whatever the template says. Refuse instead, and name the remedy.
-- +goose StatementBegin
DO $$
DECLARE
    owned integer;
BEGIN
    SELECT count(*) INTO owned
      FROM classification_rules
     WHERE ruleset_version = 'v1' AND org_id IS NOT NULL;
    IF owned <> 0 THEN
        RAISE EXCEPTION
            '% organisation-owned rules exist in ruleset v1; this migration carries template rules only', owned
        USING HINT = 'Extend the copy in 00019 to carry org-owned rules forward, then run it again.';
    END IF;
END $$;
-- +goose StatementEnd

INSERT INTO classification_rules (org_id, scope, priority, matcher, category_id,
                                  taxonomy_version, ruleset_version, active)
SELECT org_id, scope, priority, matcher, category_id, taxonomy_version, 'v2', active
  FROM classification_rules
 WHERE ruleset_version = 'v1';

INSERT INTO classification_rules (org_id, scope, priority, matcher, category_id,
                                  taxonomy_version, ruleset_version, active)
SELECT v.org_id, v.scope, v.priority, v.matcher, c.id, v.taxonomy_version, v.ruleset_version, v.active
  FROM (VALUES
  (NULL::uuid, 'country:BY', 72, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "РАЗРАБОТК"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '0101', 'v1', 'v2', true),
  (NULL::uuid, 'country:BY', 73, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ДОРАБОТК"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '0101', 'v1', 'v2', true),
  (NULL::uuid, 'country:BY', 74, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПОДДЕРЖК"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '0101', 'v1', 'v2', true)
) AS v(org_id, scope, priority, matcher, category_code, taxonomy_version, ruleset_version, active)
  JOIN categories c
    ON c.taxonomy_version = v.taxonomy_version
   AND c.org_id IS NULL
   AND c.code = v.category_code;

-- The JOIN drops a rule whose category code does not resolve rather than
-- failing, and the copy above would carry fewer rows in silence if v1 were not
-- what this migration believes it is. Count the result, the way 006 counts its
-- own seed.
-- +goose StatementBegin
DO $$
DECLARE
    v2 integer;
BEGIN
    SELECT count(*) INTO v2
      FROM classification_rules
     WHERE ruleset_version = 'v2' AND org_id IS NULL;
    IF v2 <> 74 THEN
        RAISE EXCEPTION 'ruleset v2 holds % template rules, expected 74 (71 carried forward + 3 new)', v2;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE classification_rules FORCE ROW LEVEL SECURITY;

RESET app.org_id;

-- +goose Down

ALTER TABLE classification_rules NO FORCE ROW LEVEL SECURITY;

DELETE FROM classification_rules WHERE ruleset_version = 'v2';

ALTER TABLE classification_rules FORCE ROW LEVEL SECURITY;
