"""Writes everything `build.py` computes to `eval/out/`.

Run: `python eval/emit.py`.

Four kinds of output, for four different readers:

  * `taxonomy.csv`, `templates.csv` — for a person to open and disagree with.
  * `rules.json`, `categories.json` — fixtures for `engine.py` and
    `run_eval.py`. These are also the shape the request will take when the
    engine runs in another process, so the wire format exists before the wire.
  * `seed_categories.sql`, `seed_rules.sql` — what changes 3.1 and 3.2 apply.

Nothing here is hand-edited. Re-running reproduces all six byte for byte.
"""

import csv
import json
import os
from collections import Counter

from build import (
    COMPUTED,
    OUT,
    TAXONOMY_VERSION,
    build_by_template,
    build_kz_template,
    build_pl_template,
    build_taxonomy,
)
from norm import NORMALIZE_VERSION, normalize_description


def _sql(v):
    return "NULL" if v is None else "'" + str(v).replace("'", "''") + "'"


def _matcher(rule):
    m = {
        "source_kind": "bank",
        "normalize_version": NORMALIZE_VERSION,
        "all": [
            {"field": rule["field"], "op": rule["op"], "value": rule["stored_value"]}
        ],
    }
    if rule["direction"]:
        m["all"].append(
            {"field": "direction", "op": "eq", "value": rule["direction"].lower()}
        )
    return m


def prepare():
    """Taxonomy plus the deduplicated, prioritised rule list."""
    cats, customers = build_taxonomy()
    rules = build_by_template(cats) + build_kz_template(cats) + build_pl_template(cats)

    # Rule and transaction both pass through normalisation, so two rules that
    # differ only by a bank's prefix are one rule.
    seen, kept, collapsed = set(), [], 0
    for r in rules:
        value = (
            normalize_description(r["value"])
            if r["field"] == "description"
            else r["value"]
        )
        key = (r["country"], r["field"], value, r["direction"], r["path"])
        if key in seen:
            collapsed += 1
            continue
        seen.add(key)
        r["stored_value"] = value
        kept.append(r)
    for i, r in enumerate(kept, 1):
        r["priority"] = i

    missing = [r for r in kept if r["path"] not in cats]
    if missing:
        raise SystemExit(
            "rules point at categories that do not exist: "
            + "; ".join(" > ".join(r["path"]) for r in missing)
        )
    return cats, customers, kept, collapsed


def write_taxonomy_csv(cats, customers):
    with open(
        os.path.join(OUT, "taxonomy.csv"), "w", newline="", encoding="utf-8-sig"
    ) as f:
        w = csv.writer(f, delimiter=";")
        w.writerow(
            [
                "code",
                "level",
                "scope",
                "L1",
                "L2",
                "L3",
                "L4",
                "L5",
                "name",
                "leaf",
                "in_pnl",
                "computed",
                "formula",
                "requires_allocation",
                "rules",
            ]
        )
        for path, c in sorted(cats.items(), key=lambda kv: kv[1]["code"]):
            w.writerow(
                [
                    c["code"],
                    c["level"],
                    c["scope"],
                    *path,
                    *[""] * (5 - len(path)),
                    c["name"],
                    "yes" if c["leaf"] else "",
                    "yes" if c["is_pnl"] else "no",
                    "",
                    "",
                    "yes" if c["alloc"] else "",
                    c["rules"],
                ]
            )
        for code, name, formula in COMPUTED:
            w.writerow(
                [
                    code,
                    1,
                    "global",
                    name,
                    "",
                    "",
                    "",
                    "",
                    name,
                    "",
                    "yes",
                    "yes",
                    formula,
                    "",
                    "",
                ]
            )
        w.writerow([])
        w.writerow(
            ["-- customer dimension: read from the counterparty, not from the tree --"]
        )
        for name, n in customers.most_common():
            w.writerow([name, n])


def write_templates_csv(cats, rules):
    with open(
        os.path.join(OUT, "templates.csv"), "w", newline="", encoding="utf-8-sig"
    ) as f:
        w = csv.writer(f, delimiter=";")
        w.writerow(
            [
                "priority",
                "country",
                "scope",
                "field",
                "op",
                "value",
                "direction",
                "category_code",
                "category_path",
            ]
        )
        for r in rules:
            w.writerow(
                [
                    r["priority"],
                    r["country"],
                    r["scope"],
                    r["field"],
                    r["op"],
                    r["stored_value"],
                    r["direction"],
                    cats[r["path"]]["code"],
                    " > ".join(r["path"]),
                ]
            )


def write_fixtures(cats, rules):
    bundle = {
        "taxonomy_version": TAXONOMY_VERSION,
        "normalize_version": NORMALIZE_VERSION,
        "rules": [
            {
                "priority": r["priority"],
                "country": r["country"],
                "scope": r["scope"],
                "category_code": cats[r["path"]]["code"],
                "category_path": " > ".join(r["path"]),
                "matcher": _matcher(r),
            }
            for r in rules
        ],
    }
    with open(os.path.join(OUT, "rules.json"), "w", encoding="utf-8") as f:
        json.dump(bundle, f, ensure_ascii=False, indent=1)

    catalogue = {
        c["code"]: {"path": list(p), **{k: v for k, v in c.items() if k != "code"}}
        for p, c in cats.items()
    }
    with open(os.path.join(OUT, "categories.json"), "w", encoding="utf-8") as f:
        json.dump(catalogue, f, ensure_ascii=False, indent=1)


# ruff: noqa: E501 -- the SQL below is generated output, not Python. Wrapping the
# INSERT column list to 88 characters would change the file this writes.

CATEGORIES_HEADER = """-- Category taxonomy, taxonomy_version = {tv}.
-- Generated by eval/emit.py from the founder's source files. Do not hand-edit.
--
-- Columns beyond docs/ARCHITECTURE.md 5.5, and why each one is needed:
--   scope                'global' | 'org'  -- levels 1-2 are ours and versioned;
--                                             leaves such as 'IT Park - membership'
--                                             belong to one organisation
--   org_id               uuid NULL         -- NULL when scope = 'global'
--   is_computed, formula                   -- GM, NM, CM, IBT and NI are arithmetic
--                                             over other lines and never a target
--   requires_allocation                    -- payroll is known, the department is not
BEGIN;
INSERT INTO categories (taxonomy_version, code, parent_code, scope, org_id, name,
                        level, is_leaf, is_pnl, is_computed, formula, requires_allocation) VALUES
"""

RULES_HEADER = """-- L1 template rules. Not one of these belongs to a customer.
--   BY {by} rules on payment text
--   KZ {kz} rules on КНП, the state payment-purpose code on every Kazakh bank row
--   PL {pl} rules, mostly on the bank's own `Typ operacji`
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
"""


def write_seeds(cats, rules):
    with open(os.path.join(OUT, "seed_categories.sql"), "w", encoding="utf-8") as f:
        f.write(CATEGORIES_HEADER.format(tv=TAXONOMY_VERSION))
        rows = []
        for path, c in sorted(cats.items(), key=lambda kv: kv[1]["code"]):
            parent = cats[path[:-1]]["code"] if len(path) > 1 else None
            rows.append(
                f"  ({_sql(TAXONOMY_VERSION)}, {_sql(c['code'])}, {_sql(parent)}, "
                f"{_sql(c['scope'])}, NULL, {_sql(c['name'])}, {c['level']}, "
                f"{str(c['leaf']).lower()}, {str(c['is_pnl']).lower()}, false, NULL, "
                f"{str(c['alloc']).lower()})"
            )
        for code, name, formula in COMPUTED:
            rows.append(
                f"  ({_sql(TAXONOMY_VERSION)}, {_sql(code)}, NULL, 'global', NULL, "
                f"{_sql(name)}, 1, false, true, true, {_sql(formula)}, false)"
            )
        f.write(
            ",\n".join(rows)
            + "\nON CONFLICT (taxonomy_version, code) DO NOTHING;\nCOMMIT;\n"
        )

    n = Counter(r["country"] for r in rules)
    with open(os.path.join(OUT, "seed_rules.sql"), "w", encoding="utf-8") as f:
        f.write(RULES_HEADER.format(by=n["BY"], kz=n["KZ"], pl=n["PL"]))
        rows = [
            f"  (NULL, {_sql(r['scope'])}, {r['priority']}, "
            f"{_sql(json.dumps(_matcher(r), ensure_ascii=False))}, "
            f"{_sql(cats[r['path']]['code'])}, {_sql(TAXONOMY_VERSION)}, true)"
            for r in rules
        ]
        f.write(",\n".join(rows) + ";\nCOMMIT;\n")


def main():
    os.makedirs(OUT, exist_ok=True)
    cats, customers, rules, collapsed = prepare()
    write_taxonomy_csv(cats, customers)
    write_templates_csv(cats, rules)
    write_fixtures(cats, rules)
    write_seeds(cats, rules)

    leaves = sum(1 for c in cats.values() if c["leaf"])
    globals_ = sum(1 for c in cats.values() if c["scope"] == "global")
    per_country = Counter(r["country"] for r in rules)
    print(
        f"taxonomy   {len(cats)} nodes (+{len(COMPUTED)} computed), {leaves} leaves, "
        f"{globals_} global / {len(cats) - globals_} org"
    )
    print(f"customers  {len(customers)} lifted out of the tree into a dimension")
    print(
        f"rules      {len(rules)} ("
        + " + ".join(f"{n} {c}" for c, n in sorted(per_country.items()))
        + f"), {collapsed} collapsed by normalisation"
    )
    print(f"written to {OUT}")


if __name__ == "__main__":
    main()
