-- Classification rules and vendor memory. Change 3.2, capability
-- `classification-engine`.
--
-- Two tables with deliberately different shapes, and the difference is the
-- point.
--
-- `classification_rules` repeats the shared-and-tenant shape migration 005
-- introduced, because most rules belong to a country and a bank rather than to
-- a customer. Measured on the founder's own statements: 41 Belarusian rules
-- reach 87.5% of rows, 21 Kazakh rules reach 86.1% of rows and 99.5% of amount,
-- and not one of them names the company they were measured on. A schema where
-- every rule has an owning organisation cannot store them.
--
-- `vendors` is an ordinary tenant table. Memory is earned by one organisation's
-- own decisions in its own review queue, and is shared with nobody. Sharing it
-- across customers is BACKLOG B-6 and a consent question, not a schema one.

-- +goose Up

CREATE TABLE classification_rules (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    taxonomy_version text        NOT NULL,

    -- Versioned apart from the taxonomy: a rule can be corrected without
    -- redrawing the tree, and a report pins both so that March reproduces in
    -- June whichever of them moved.
    ruleset_version  text        NOT NULL,

    -- NULL = a template rule, used by every organisation.
    org_id           uuid        NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    scope            text        NOT NULL,   -- 'country:BY' | 'bank:priorbank' | 'org' | ...

    priority         integer     NOT NULL,
    matcher          jsonb       NOT NULL,
    category_id      uuid        NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    active           boolean     NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT rules_scope_matches_owner CHECK (
        (org_id IS NULL AND scope <> 'org') OR (org_id IS NOT NULL AND scope = 'org')),

    -- A matcher is an AND over named fields. Asserted here rather than
    -- trusted, because a matcher with no conditions would fire on every
    -- transaction.
    --
    -- coalesce, not a bare comparison: `'{}'::jsonb -> 'all'` is SQL NULL,
    -- jsonb_typeof(NULL) is NULL, and a CHECK whose result is NULL passes.
    -- A matcher with no `all` key at all is exactly the shape this constraint
    -- exists to refuse, and without the coalesce it is the one shape that
    -- gets through. A test asserts all three spellings.
    CONSTRAINT rules_matcher_has_conditions CHECK (
        coalesce(jsonb_typeof(matcher -> 'all'), '') = 'array'
        AND jsonb_array_length(matcher -> 'all') > 0)
);

-- Priority is unique per owner: within the template set, and within each
-- organisation's own set. It is deliberately NOT unique across the two, and
-- the reason is that a per-org rule exists to overrule a template. Making the
-- numbers globally unique would force a customer to know which template
-- priorities are taken before writing a rule, and would still not say which
-- of the two wins. The ordering does: EffectiveRules in
-- core/internal/db/query/classify.sql puts an organisation's own rules ahead
-- of every template rule, and only then sorts by priority. So a tie the index
-- allows is still not a tie the engine sees.
CREATE UNIQUE INDEX rules_priority_idx
    ON classification_rules (taxonomy_version, ruleset_version, org_id, priority)
    NULLS NOT DISTINCT;

CREATE INDEX rules_effective_idx
    ON classification_rules (taxonomy_version, ruleset_version, org_id, priority)
    WHERE active;

-- The same problem categories.parent_id has, for the same reason: a template
-- rule (org_id NULL) points at a shared category (org_id NULL), and an
-- organisation's own rule may point at a shared category or at its own. A
-- composite foreign key cannot say "shared or mine", so a trigger says it --
-- and raises the identical error whether the category belongs to somebody else
-- or does not exist, which is what keeps a foreign key from reporting the
-- existence of a row the caller may not read.
-- +goose StatementBegin
CREATE FUNCTION rules_category_is_visible() RETURNS trigger
    LANGUAGE plpgsql
    AS $fn$
DECLARE
    cat_org   uuid;
    cat_leaf  boolean;
    cat_calc  boolean;
    found     boolean;
BEGIN
    SELECT org_id, is_leaf, is_computed, true
      INTO cat_org, cat_leaf, cat_calc, found
      FROM categories WHERE id = NEW.category_id;

    IF NOT coalesce(found, false)
       OR (cat_org IS NOT NULL AND cat_org IS DISTINCT FROM NEW.org_id) THEN
        RAISE EXCEPTION 'category % is not visible to this rule', NEW.category_id
            USING ERRCODE = '23503';
    END IF;

    -- A rule that targets a section double-counts: the section's figure is
    -- already the sum of its children. A rule that targets a computed line is
    -- worse -- GM is NET SALES minus CS, so the amount would appear in the
    -- formula and again in its result.
    IF NOT cat_leaf OR cat_calc THEN
        RAISE EXCEPTION 'category % is not classifiable: a rule may target only a leaf that is not computed',
            NEW.category_id USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$fn$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER rules_category_is_visible
    AFTER INSERT OR UPDATE OF category_id, org_id ON classification_rules
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION rules_category_is_visible();

-- ---------------------------------------------------------------------------
-- Vendor memory. An ordinary tenant table.
-- ---------------------------------------------------------------------------

CREATE TABLE vendors (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,

    -- Produced by core's counterparty_key(): 'tax:220340017991' when the
    -- statement carried a tax identifier, 'name:ДЖОНДОРИ' when it did not.
    key          text        NOT NULL,

    -- The version of the function that produced the key. Changing that
    -- function changes what counts as the same counterparty, which is a
    -- backfill of this column and never a silent reinterpretation of what a
    -- customer already approved.
    key_version  text        NOT NULL,

    display_name text        NOT NULL,
    category_id  uuid        NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,

    -- Who decided, and when. A memory row is a human's answer, and the review
    -- queue is where it comes from.
    decided_by   uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendors_key_unique UNIQUE (org_id, key_version, key)
);

CREATE INDEX vendors_lookup_idx ON vendors (org_id, key_version, key);

CREATE CONSTRAINT TRIGGER vendors_category_is_visible
    AFTER INSERT OR UPDATE OF category_id, org_id ON vendors
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION rules_category_is_visible();

-- ---------------------------------------------------------------------------
-- The template rules, seeded before row-level security is enabled on this
-- table, for the reason migration 005 gives at length: FORCE subjects the
-- owner to the policies, and vekst_migrator is not a superuser.
--
-- The new part is the other direction. A rule names its category by code, so
-- the seed has to READ `categories` -- and so does the trigger above, on every
-- row it inserts. `categories` was already forced by migration 005, and its
-- read policy calls app_current_org(), which raises 42704 when no tenant
-- context is set. Ordering cannot help here: the table this migration reads
-- was locked one migration ago.
--
-- So the migration opens a tenant context and picks the one organisation that
-- can never exist. The nil UUID belongs to nobody, which makes
-- `org_id IS NULL OR org_id = app_current_org()` collapse to exactly the
-- shared rows -- the only rows a template rule is allowed to point at anyway.
-- It is the honest reading: this migration is reading the taxonomy as no
-- customer. RESET below puts fail-closed back before anything else runs.
-- ---------------------------------------------------------------------------

SET LOCAL app.org_id = '00000000-0000-0000-0000-000000000000';

INSERT INTO classification_rules (org_id, scope, priority, matcher, category_id,
                                  taxonomy_version, ruleset_version, active)
SELECT v.org_id, v.scope, v.priority, v.matcher, c.id, v.taxonomy_version, v.ruleset_version, v.active
  FROM (VALUES
  (NULL::uuid, 'country:BY', 1, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ВЫПЛАТЫ ПО ДОГОВОРУ ВОЗМЕЗДНОГО ОКАЗАНИЯ УСЛУГ; СОГЛ. СПИСКУ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 2, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ВЫПЛАТЫ ПО ДОГОВОРАМ ПОДРЯДА; СОГЛ. СПИСКУ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 3, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПРОФЕССИОНАЛЬНЫЙ ДОХОД;СОГЛ. СПИСКУ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 4, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПОДОХОДНЫЙ НАЛОГ;ИЗ ДИВИДЕНДОВ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '09', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 5, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЛАТА ЗА ЗАЧИСЛЕНИЕ СТРАХОВЫХ ВЫПЛАТ,ДИВИДЕНДОВ,АРЕНДЫ,ЗАЙМОВ,ДРУГИХ ВЫПЛАТ И ПЕРЕЧИСЛЕНИЙ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401010204', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 6, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЛАТА ЗА БЕЗНАЛ.ЗАЧИСЛ.ЗП И ПЛАТЕЖЕЙ К НЕЙ ПРИРАВ.НА КАРТ-СЧЕТА В НАЦ. ВАЛЮТЕ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401010204', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 7, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ЕЖЕМЕСЯЧНАЯ АБОНЕНТСКАЯ ПЛАТА ЗА ПАКЕТ УСЛУГ \"MS BUSINESS DIRECT\""}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401010204', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 8, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЛАТА ЗА УСЛУГИ \"ПРИОРБАНК\" ОАО"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401010204', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 9, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "СТРАХОВЫЕ ВЗНОСЫ ПО ОБЯЗАТЕЛЬНОМУ СТРАХОВАНИЮ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 10, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ВЫПЛАТА ПО АВАНСОВОМУ ОТЧЕТУ ХОЗ.РАСХОДЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '050101', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 11, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "УПЛАТА ПРОЦЕНТОВ ПО ОСТАТКАМ НА СЧЕТЕ"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '060201', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 12, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЕРЕВОД С ПРОДАЖЕЙ ПО БАЗОВОМУ КУРСУ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '060101', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 13, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЛАТА ЗА ЗАЧИСЛЕНИЕ ВАЛЮТНЫХ СРЕДСТВ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401010204', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 14, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ВОЗВРАТ ОШИБОЧНО ПЕРЕВЕДЕННОЙ СУММЫ"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '050202', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 15, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПРЕДОПЛАТА ПО ДОГОВОРУ АРЕНДЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401020208', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 16, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПОСОБИЕ ПО УХОДУ ЗА РЕБЕНКОМ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 17, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПОСОБИЯ ПО УХОДУ ЗА РЕБЕНКОМ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 18, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ВОЗВРАТ ИЗЛИШНЕ ПЕРЕЧИСЛЕНН"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '050202', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 19, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОПЛАТА ПО ДОГОВОРУ АРЕНДЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401020208', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 20, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОПЛАТА ПОДОХОДНОГО НАЛОГА"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 21, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "СВОБОДНАЯ ПРОДАЖА ВАЛЮТЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '060101', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 22, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОПЛАТА НАЛОГА НА ПРИБЫЛЬ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '07', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 23, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "СВОБОДНАЯ ПРОДАЖА ВАЛЮТЫ"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '060101', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 24, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "КОМАНДИРОВОЧНЫЕ РАСХОДЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 25, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "РАСЧЕТ ПРИ УВОЛЬНЕНИИ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 26, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОКОНЧАТЕЛЬНЫЙ РАСЧЕТ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 27, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЕРЕВОД С КОНВЕРСИЕЙ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '060101', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 28, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "МАТЕРИАЛЬНАЯ ПОМОЩЬ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 29, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОНОЧАТЕЛЬНЫЙ РАСЧЕТ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 30, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОТЧИСЛЕНИЯ В ФСЗН"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 31, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ЗАРАБОТНАЯ ПЛАТА"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 32, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПОДОХОДНЫЙ НАЛОГ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 33, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ДЕНЕЖНАЯ ПОМОЩЬ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 34, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "СБОР ЗА РЕКЛАМУ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '05010302', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 35, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЕРЕОЦЕНКА"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '060102', 'v1', 'v1', true),
  (NULL::uuid, 'bank:priorbank', 36, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЕРЕОЦЕНКА"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '060102', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 37, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "БОЛЬНИЧНЫЙ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 38, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ДИВИДЕНДЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '09', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 39, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОТПУСКНЫЕ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 40, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОПЛ.ПТ"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'country:BY', 41, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "АВАНС"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 42, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "010"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 43, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "012"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 44, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "089"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 45, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "120"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '05010304', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 46, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "121"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 47, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "122"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 48, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "223"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '060101', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 49, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "223"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '060101', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 50, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "230"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '060101', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 51, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "332"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0404', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 52, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "661"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '09', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 53, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "841"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401010204', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 54, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "851"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '0101', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 55, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "852"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401020202', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 56, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "854"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401040101', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 57, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "855"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401010201', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 58, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "857"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '0101', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 59, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "858"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '0101', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 60, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "859"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '0101', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 61, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "911"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'country:KZ', 62, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "912"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '050104', 'v1', 'v1', true),
  (NULL::uuid, 'bank:pkobp', 63, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "Prowizja"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401010204', 'v1', 'v1', true),
  (NULL::uuid, 'bank:pkobp', 64, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "Opłata"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401010204', 'v1', 'v1', true),
  (NULL::uuid, 'bank:pkobp', 65, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "Opłata za użytkowanie karty"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0401010204', 'v1', 'v1', true),
  (NULL::uuid, 'bank:pkobp', 66, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "Przelew do ZUS"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '0403', 'v1', 'v1', true),
  (NULL::uuid, 'bank:pkobp', 67, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "Przelew do US VAT"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '05010305', 'v1', 'v1', true),
  (NULL::uuid, 'bank:pkobp', 68, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "Uznanie"}, {"field": "direction", "op": "eq", "value": "income"}]}'::jsonb, '060101', 'v1', 'v1', true),
  (NULL::uuid, 'bank:pkobp', 69, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "regulated_code", "op": "eq", "value": "Obciążenie"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '060101', 'v1', 'v1', true),
  (NULL::uuid, 'country:PL', 70, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "WARTOŚĆ VAT"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '05010305', 'v1', 'v1', true),
  (NULL::uuid, 'country:PL', 71, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ZALICZKA NA CIT"}, {"field": "direction", "op": "eq", "value": "expense"}]}'::jsonb, '07', 'v1', 'v1', true)
) AS v(org_id, scope, priority, matcher, category_code, taxonomy_version, ruleset_version, active)
  JOIN categories c
    ON c.taxonomy_version = v.taxonomy_version
   AND c.org_id IS NULL
   AND c.code = v.category_code;

-- The JOIN above drops a rule whose category code no longer exists rather than
-- failing, which would seed a smaller ruleset in silence. Count it.
-- +goose StatementBegin
DO $$
DECLARE
    seeded integer;
BEGIN
    SELECT count(*) INTO seeded FROM classification_rules WHERE org_id IS NULL;
    IF seeded <> 71 THEN
        RAISE EXCEPTION 'seeded % template rules, expected 71 -- a category code did not resolve', seeded;
    END IF;
END $$;
-- +goose StatementEnd

RESET app.org_id;

ALTER TABLE classification_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE classification_rules FORCE ROW LEVEL SECURITY;

CREATE POLICY classification_rules_read ON classification_rules FOR SELECT
    USING (org_id IS NULL OR org_id = app_current_org());

CREATE POLICY classification_rules_write ON classification_rules FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

ALTER TABLE vendors ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendors FORCE ROW LEVEL SECURITY;

CREATE POLICY vendors_tenant ON vendors FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- +goose Down

DROP TABLE vendors;
DROP TABLE classification_rules;
DROP FUNCTION rules_category_is_visible();
