-- L1 template rules. Not one of these belongs to a customer.
--   BY 41 rules on payment text
--   KZ 21 rules on КНП, the state payment-purpose code on every Kazakh bank row
--   PL 9 rules, mostly on the bank's own `Typ operacji`
--
-- Measured against real statements for 2023-2024 by eval/run_eval.py:
--   BY  87.5% of rows / 85.2% of amount   accuracy 94.8%, 15 disagreements
--   KZ  86.1% of rows / 99.5% of amount   accuracy 92.4%,  4 disagreements
--   PL  38.6% of rows / 42.5% of amount   accuracy 99.6%,  1 disagreement
--   20 disagreements with the accountant's own labelling across 4508 rows.
--
-- Poland's coverage is low because of the source, not the rules: card payments
-- are 214 of its 735 rows and were never categorised, and its revenue is
-- identified by who paid -- vendor memory, not a country rule.
--
-- Columns beyond docs/ARCHITECTURE.md 5.5:
--   org_id  uuid NULL  -- NULL = a template rule, shared by every organisation
--   scope   text       -- 'country:BY' | 'bank:priorbank' | 'country:KZ' |
--                      -- 'country:PL' | 'bank:pkobp' | 'org'
-- Without them there is nowhere to store the rules that do most of the work.
BEGIN;
INSERT INTO classification_rules (org_id, scope, priority, matcher, category_code,
                                  taxonomy_version, active) VALUES
  (NULL, 'country:BY', 1, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ВЫПЛАТЫ ПО ДОГОВОРУ ВОЗМЕЗДНОГО ОКАЗАНИЯ УСЛУГ; СОГЛ. СПИСКУ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 2, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ВЫПЛАТЫ ПО ДОГОВОРАМ ПОДРЯДА; СОГЛ. СПИСКУ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 3, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПРОФЕССИОНАЛЬНЫЙ ДОХОД;СОГЛ. СПИСКУ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 4, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПОДОХОДНЫЙ НАЛОГ;ИЗ ДИВИДЕНДОВ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '09', 'v1', true),
  (NULL, 'bank:priorbank', 5, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЛАТА ЗА ЗАЧИСЛЕНИЕ СТРАХОВЫХ ВЫПЛАТ,ДИВИДЕНДОВ,АРЕНДЫ,ЗАЙМОВ,ДРУГИХ ВЫПЛАТ И ПЕРЕЧИСЛЕНИЙ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401010204', 'v1', true),
  (NULL, 'bank:priorbank', 6, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЛАТА ЗА БЕЗНАЛ.ЗАЧИСЛ.ЗП И ПЛАТЕЖЕЙ К НЕЙ ПРИРАВ.НА КАРТ-СЧЕТА В НАЦ. ВАЛЮТЕ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401010204', 'v1', true),
  (NULL, 'bank:priorbank', 7, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ЕЖЕМЕСЯЧНАЯ АБОНЕНТСКАЯ ПЛАТА ЗА ПАКЕТ УСЛУГ \"MS BUSINESS DIRECT\""}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401010204', 'v1', true),
  (NULL, 'bank:priorbank', 8, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЛАТА ЗА УСЛУГИ \"ПРИОРБАНК\" ОАО"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401010204', 'v1', true),
  (NULL, 'country:BY', 9, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "СТРАХОВЫЕ ВЗНОСЫ ПО ОБЯЗАТЕЛЬНОМУ СТРАХОВАНИЮ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'country:BY', 10, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ВЫПЛАТА ПО АВАНСОВОМУ ОТЧЕТУ ХОЗ.РАСХОДЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '050101', 'v1', true),
  (NULL, 'bank:priorbank', 11, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "УПЛАТА ПРОЦЕНТОВ ПО ОСТАТКАМ НА СЧЕТЕ"}, {"field": "direction", "op": "eq", "value": "income"}]}', '060201', 'v1', true),
  (NULL, 'bank:priorbank', 12, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЕРЕВОД С ПРОДАЖЕЙ ПО БАЗОВОМУ КУРСУ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '060101', 'v1', true),
  (NULL, 'bank:priorbank', 13, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЛАТА ЗА ЗАЧИСЛЕНИЕ ВАЛЮТНЫХ СРЕДСТВ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401010204', 'v1', true),
  (NULL, 'country:BY', 14, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ВОЗВРАТ ОШИБОЧНО ПЕРЕВЕДЕННОЙ СУММЫ"}, {"field": "direction", "op": "eq", "value": "income"}]}', '050202', 'v1', true),
  (NULL, 'country:BY', 15, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПРЕДОПЛАТА ПО ДОГОВОРУ АРЕНДЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401020208', 'v1', true),
  (NULL, 'country:BY', 16, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПОСОБИЕ ПО УХОДУ ЗА РЕБЕНКОМ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 17, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПОСОБИЯ ПО УХОДУ ЗА РЕБЕНКОМ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 18, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ВОЗВРАТ ИЗЛИШНЕ ПЕРЕЧИСЛЕНН"}, {"field": "direction", "op": "eq", "value": "income"}]}', '050202', 'v1', true),
  (NULL, 'country:BY', 19, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОПЛАТА ПО ДОГОВОРУ АРЕНДЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401020208', 'v1', true),
  (NULL, 'country:BY', 20, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОПЛАТА ПОДОХОДНОГО НАЛОГА"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'bank:priorbank', 21, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "СВОБОДНАЯ ПРОДАЖА ВАЛЮТЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '060101', 'v1', true),
  (NULL, 'country:BY', 22, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОПЛАТА НАЛОГА НА ПРИБЫЛЬ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '07', 'v1', true),
  (NULL, 'bank:priorbank', 23, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "СВОБОДНАЯ ПРОДАЖА ВАЛЮТЫ"}, {"field": "direction", "op": "eq", "value": "income"}]}', '060101', 'v1', true),
  (NULL, 'country:BY', 24, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "КОМАНДИРОВОЧНЫЕ РАСХОДЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 25, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "РАСЧЕТ ПРИ УВОЛЬНЕНИИ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 26, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОКОНЧАТЕЛЬНЫЙ РАСЧЕТ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'bank:priorbank', 27, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЕРЕВОД С КОНВЕРСИЕЙ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '060101', 'v1', true),
  (NULL, 'country:BY', 28, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "МАТЕРИАЛЬНАЯ ПОМОЩЬ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 29, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОНОЧАТЕЛЬНЫЙ РАСЧЕТ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 30, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОТЧИСЛЕНИЯ В ФСЗН"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'country:BY', 31, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ЗАРАБОТНАЯ ПЛАТА"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 32, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПОДОХОДНЫЙ НАЛОГ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'country:BY', 33, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ДЕНЕЖНАЯ ПОМОЩЬ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 34, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "СБОР ЗА РЕКЛАМУ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '05010302', 'v1', true),
  (NULL, 'bank:priorbank', 35, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЕРЕОЦЕНКА"}, {"field": "direction", "op": "eq", "value": "income"}]}', '060102', 'v1', true),
  (NULL, 'bank:priorbank', 36, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ПЕРЕОЦЕНКА"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '060102', 'v1', true),
  (NULL, 'country:BY', 37, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "БОЛЬНИЧНЫЙ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 38, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ДИВИДЕНДЫ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '09', 'v1', true),
  (NULL, 'country:BY', 39, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОТПУСКНЫЕ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:BY', 40, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ОПЛ.ПТ"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'country:BY', 41, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "АВАНС"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:KZ', 42, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "010"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'country:KZ', 43, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "012"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'country:KZ', 44, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "089"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'country:KZ', 45, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "120"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '05010304', 'v1', true),
  (NULL, 'country:KZ', 46, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "121"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'country:KZ', 47, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "122"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'country:KZ', 48, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "223"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '060101', 'v1', true),
  (NULL, 'country:KZ', 49, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "223"}, {"field": "direction", "op": "eq", "value": "income"}]}', '060101', 'v1', true),
  (NULL, 'country:KZ', 50, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "230"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '060101', 'v1', true),
  (NULL, 'country:KZ', 51, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "332"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0404', 'v1', true),
  (NULL, 'country:KZ', 52, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "661"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '09', 'v1', true),
  (NULL, 'country:KZ', 53, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "841"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401010204', 'v1', true),
  (NULL, 'country:KZ', 54, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "851"}, {"field": "direction", "op": "eq", "value": "income"}]}', '0101', 'v1', true),
  (NULL, 'country:KZ', 55, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "852"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401020202', 'v1', true),
  (NULL, 'country:KZ', 56, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "854"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401040101', 'v1', true),
  (NULL, 'country:KZ', 57, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "855"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401010201', 'v1', true),
  (NULL, 'country:KZ', 58, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "857"}, {"field": "direction", "op": "eq", "value": "income"}]}', '0101', 'v1', true),
  (NULL, 'country:KZ', 59, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "858"}, {"field": "direction", "op": "eq", "value": "income"}]}', '0101', 'v1', true),
  (NULL, 'country:KZ', 60, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "859"}, {"field": "direction", "op": "eq", "value": "income"}]}', '0101', 'v1', true),
  (NULL, 'country:KZ', 61, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "911"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'country:KZ', 62, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "knp", "op": "eq", "value": "912"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '050104', 'v1', true),
  (NULL, 'bank:pkobp', 63, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "operation_type", "op": "eq", "value": "Prowizja"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401010204', 'v1', true),
  (NULL, 'bank:pkobp', 64, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "operation_type", "op": "eq", "value": "Opłata"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401010204', 'v1', true),
  (NULL, 'bank:pkobp', 65, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "operation_type", "op": "eq", "value": "Opłata za użytkowanie karty"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0401010204', 'v1', true),
  (NULL, 'bank:pkobp', 66, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "operation_type", "op": "eq", "value": "Przelew do ZUS"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '0403', 'v1', true),
  (NULL, 'bank:pkobp', 67, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "operation_type", "op": "eq", "value": "Przelew do US VAT"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '05010305', 'v1', true),
  (NULL, 'bank:pkobp', 68, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "operation_type", "op": "eq", "value": "Uznanie"}, {"field": "direction", "op": "eq", "value": "income"}]}', '060101', 'v1', true),
  (NULL, 'bank:pkobp', 69, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "operation_type", "op": "eq", "value": "Obciążenie"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '060101', 'v1', true),
  (NULL, 'country:PL', 70, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "WARTOŚĆ VAT"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '05010305', 'v1', true),
  (NULL, 'country:PL', 71, '{"source_kind": "bank", "normalize_version": "v1", "all": [{"field": "description", "op": "contains_all", "value": "ZALICZKA NA CIT"}, {"field": "direction", "op": "eq", "value": "expense"}]}', '07', 'v1', true);
COMMIT;
