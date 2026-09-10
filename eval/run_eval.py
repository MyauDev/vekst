"""Measures the templates against real statements. One command, three countries.

Run: `python eval/run_eval.py`.

Why this exists: without it, every change to a rule is a hope. `BACKLOG.md` B-1
also makes a measured unmatched rate the condition for building a model layer,
and nothing else in the repository produces that number.

The reference labelling is the accountant's own categorisation file — the only
labelling that exists. Disagreeing with it is not automatically an error, so
the report separates three cases:

  agreed              same category.
  awaiting allocation we said 'PAYROLL to distribute', they named a department.
                      КНП 332 knows it is wages; it cannot know whose.
  less specific       our answer is an ancestor of theirs. Cautious, not wrong.
  DISAGREED           different branches. This is the number to watch.

The first version of this harness collapsed the middle two into "wrong" and
reported 198 failures where there were 19. A metric that counts caution as
failure makes you optimise the wrong thing.
"""

import json
import os
from collections import Counter

from build import build_taxonomy, canon
from engine import ENGINE_VERSION, classify
from norm import NORMALIZE_VERSION, counterparty_key, normalize_description
from sources import STATEMENTS, load_rules

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "out")


def _up(s: str) -> str:
    return normalize_description(s)


def reference_labeller(country: str, cats: dict):
    """The accountant's rules, applied in the order they sit in the file."""
    rules = load_rules(country)

    def haystack(rule_value: str, txn) -> str:
        """Compare a rule's value against the field its own prefix names.

        The Polish file packs several fields into one cell, so a plain search
        over the whole cell matches the wrong one: `Rachunek kontrahenta:`
        (the counterparty) and `Rachunek:` (the customer's own account) both
        hold account numbers, and searching the blob labelled 22 payments to
        the social-insurance office as currency conversions.
        """
        low = rule_value.lower()
        if low.startswith("rachunek kontrahenta"):
            return _up(txn.get("acct", ""))
        if low.startswith(("tytuł", "tytul")):
            return _up(txn.get("purp", ""))
        return _up(txn.get("purp", "")) + " || " + _up(txn.get("cp", ""))

    def label(txn):
        for r in rules:
            if r["cp"] and _up(r["cp"]) not in _up(txn.get("cp", "")):
                continue
            parts = [p.strip() for p in _up(r["purp"]).split(";") if p.strip()]
            if parts and not all(p in haystack(r["purp"], txn) for p in parts):
                continue
            if r["dir"] and r["dir"] != txn["dir"]:
                continue
            path = canon(r["path"])
            if path[:2] == ("NET SALES", "SOFTWARE DEVELOPMENT"):
                path = path[:2]
            return cats[path]["code"] if path in cats else None
        return None

    return label


def report(country: str, txns: list, rules: list, cats: dict, code2path: dict) -> dict:
    mine = [r for r in rules if r["country"] == country]
    label = reference_labeller(country, cats)
    allocation_codes = {c["code"] for c in cats.values() if c["alloc"]}
    total_amount = sum(t["amt"] for t in txns) or 1.0

    def is_ancestor(ours: str, theirs: str) -> bool:
        a, b = code2path.get(ours), code2path.get(theirs)
        if not a or not b:
            return False
        # A trailing 'Other' exists only because a node was both leaf and
        # section; it does not make the node deeper in any meaningful sense.
        if a[-1] == "Other":
            a = a[:-1]
        return len(a) < len(b) and b[: len(a)] == a

    fired, matched, missed = Counter(), [], []
    agreed = awaiting = less = disagreed = unlabelled = 0
    conflicts = []

    for t in txns:
        res = classify(t, mine)
        if not res:
            missed.append(t)
            continue
        matched.append(t)
        fired[res["rule"]] += 1
        truth = label(t)
        if truth is None:
            unlabelled += 1
        elif truth == res["category"]:
            agreed += 1
        elif res["category"] in allocation_codes:
            awaiting += 1
        elif is_ancestor(res["category"], truth):
            less += 1
        else:
            disagreed += 1
            conflicts.append((t, res["category"], truth))

    print(f"\n{'=' * 78}")
    print(f"{country}   rules {len(mine)}   transactions {len(txns)}")
    rows_pct = len(matched) / len(txns) * 100
    amount_pct = sum(t["amt"] for t in matched) / total_amount * 100
    print(
        f"  COVERAGE  {len(matched):5}/{len(txns)} = {rows_pct:5.1f}% of rows"
        f"   {amount_pct:5.1f}% of amount"
    )
    judged = agreed + awaiting + less + disagreed
    if judged:
        print(
            f"  ACCURACY  agreed {agreed:5} ({agreed / judged * 100:5.1f}%)"
            f"   awaiting allocation {awaiting:4}   less specific {less:3}"
            f"   DISAGREED {disagreed:3} ({disagreed / judged * 100:5.2f}%)"
        )
    dead = [r for r in mine if not fired.get(r["priority"])]
    fired_count = len(mine) - len(dead)
    print(f"  RULES     fired {fired_count}/{len(mine)}   never fired {len(dead)}")

    if missed:
        keys = {
            counterparty_key(t.get("cp", ""), t.get("tax", ""), t.get("acct", ""))[0]
            for t in missed
        }
        print(
            f"  RESIDUE   {len(missed)} rows over {len(keys)} counterparties"
            f"  — one review decision each, then L0 has them"
        )
        agg = Counter()
        for t in missed:
            agg[(t.get("cp") or t.get("purp") or "")[:52]] += 1
        for name, n in agg.most_common(4):
            print(f"            {n:4}  {name}")

    for t, ours, theirs in conflicts[:3]:
        print(f"  ! {' > '.join(code2path[ours]):<44} <- ours")
        theirs_path = " > ".join(code2path[theirs])
        print(f"    {theirs_path:<44} <- theirs   {t.get('purp', '')[:52]}")

    return {
        "matched": len(matched),
        "total": len(txns),
        "disagreed": disagreed,
        "dead": len(dead),
        "residue": len(missed),
    }


def main():
    with open(os.path.join(OUT, "rules.json"), encoding="utf-8") as f:
        bundle = json.load(f)
    cats, _ = build_taxonomy()
    code2path = {c["code"]: p for p, c in cats.items()}

    print(
        f"taxonomy={bundle['taxonomy_version']}  engine={ENGINE_VERSION}  "
        f"normalize={NORMALIZE_VERSION}"
    )

    totals = Counter()
    for country, load in STATEMENTS.items():
        r = report(country, load(), bundle["rules"], cats, code2path)
        for k, v in r.items():
            totals[k] += v

    print(f"\n{'=' * 78}")
    print(
        f"TOTAL  coverage {totals['matched']}/{totals['total']} = "
        f"{totals['matched'] / totals['total'] * 100:.1f}%"
        f"   disagreements {totals['disagreed']}"
        f"   rules that never fired {totals['dead']}"
    )


if __name__ == "__main__":
    main()
