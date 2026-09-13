"""Emit the fixtures that pin the two ports of this directory to this directory.

`normalize_conformance.json` pins `core/internal/normalize` (Go) to `norm.py`.
`engine_conformance.json` pins `vekst_classifier.engine` (the service) to
`engine.py`.

There are two implementations of normalisation, in two languages, and change
3.2's design says this one is the reference: it is what the harness measured
the rule set with, so the accuracy numbers are only true of the text it
produced. The same holds for the engine: `run_eval.py` measures `engine.py`, so the
coverage and accuracy numbers in the design describe that code and not the
service's copy of it. Both ports replay what this script writes and fail on any
difference.

Two deliberate choices about the corpus:

  * It is read from `core/testdata/priorbank-by/`, the redacted fixtures, and
    never from `../docCl`. The founder's own statements do not enter the
    repository, and CI cannot see them anyway -- a conformance test that
    needed them would simply not run.

  * The redacted rows are joined by hand-written cases. Anonymisation is good
    at keeping row shapes and bad at keeping rare ones: nothing in four months
    of one Belarusian company's statements exercises a Polish legal form, an
    address marker, or a name that begins with a digit. Those are listed
    below, each with the behaviour it pins.

Run from the repository root:

    python eval/conformance.py
"""

import csv
import glob
import json
import os
import pathlib
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from engine import ENGINE_VERSION, classify
from norm import NORMALIZE_VERSION, counterparty_key, normalize_description

FIXTURES = pathlib.Path("core/testdata/priorbank-by")
OUT = pathlib.Path("eval/out/normalize_conformance.json")
ENGINE_OUT = pathlib.Path("eval/out/engine_conformance.json")
RULES = pathlib.Path("eval/out/rules.json")


def redacted_rows() -> list[dict[str, str]]:
    """Every transaction row in the committed Priorbank fixtures.

    The column map is read from the header rather than fixed, for the reason
    `sources.by_txns` gives: a rouble account carries `Корреспондент.УНП` and
    a currency account does not.
    """
    rows: list[dict[str, str]] = []
    for f in sorted(glob.glob(str(FIXTURES / "*.csv"))):
        with open(f, encoding="cp1251") as fh:
            raw = list(csv.reader(fh, delimiter=";"))
        hi = next(
            (i for i, r in enumerate(raw) if r and r[0].startswith("Дата док")), None
        )
        if hi is None:
            continue
        head = {n.strip(): i for i, n in enumerate(raw[hi])}

        def cell(r: list[str], name: str, head: dict[str, int] = head) -> str:
            i = head.get(name)
            return (r[i] or "").strip() if i is not None and i < len(r) else ""

        for r in raw[hi + 1 :]:
            if len(r) < 5 or not re.match(r"^\d{2}\.\d{2}\.\d{4}$", (r[0] or "").strip()):
                continue
            debit = cell(r, "Номинал.Дебет").replace(" ", "").replace(" ", "")
            rows.append(
                {
                    "name": cell(r, "Корреспондент.Название"),
                    "tax_id": cell(r, "Корреспондент.УНП"),
                    "account": cell(r, "Корреспондент.Счет"),
                    "description": cell(r, "Назначение"),
                    "direction": "Expense" if _num(debit) > 0 else "Income",
                }
            )
    return rows


def _num(s: str) -> float:
    try:
        return float(s.replace(",", "."))
    except ValueError:
        return 0.0


# Descriptions no redacted statement happens to contain. Each line is here
# because it is the only thing that exercises one branch.
EXTRA_DESCRIPTIONS = [
    "",
    "   ",
    # The noise prefixes, alone and combined.
    "Частичная оплата. Оплата по счету 15 от 01.03.2025",
    "Rachunek kontrahenta: 03 5954 0014 67 TYTUL: FAKTURA 7/2025",
    "/ROC/NONREF/PURP/OTHR/URI/INVOICE 12",
    # Digit-group joining, and the cases that must NOT join: a run of
    # separators with no digit after it, and a hyphen between letters.
    "PL 61 1090 1014 0000 0712 1981 2874",
    "СЧЕТ 12 - 34 ОТ 05 - 2025",
    "INVOICE 12 -  AB",
    "CO-OPERATIVE 12-AB",
    # NFD input, as macOS exports it: "й" as two code points.
    "ЗАРАБОТНАЯ ПЛАТА ЗА МАЙ",
    "ЗАРАБОТНАЯ ПЛАТА ЗА МАЙ",
    # Quote folding and full uppercase mapping.
    "оплата по договору «Ромашка» от 01.01.2025",
    "Zahlung für Straße 12",
    # A no-break space, which NFKC turns into an ordinary one.
    "ОПЛАТА ПО СЧЕТУ",
]

# Counterparty triples, likewise. (name, tax_id, account).
EXTRA_KEYS = [
    ("", "", ""),
    # The tiers, and the boundary between them: eight digits is a key, seven
    # is not, and a column of zeroes is how these exports spell "absent".
    ("ООО РОМАШКА", "220340017991", "BY77PJCB30120097991000000933"),
    ("ООО РОМАШКА", "1234567", "BY77PJCB30120097991000000933"),
    ("ООО РОМАШКА", "12345678", ""),
    ("ООО РОМАШКА", "0", ""),
    ("ООО РОМАШКА", "000000000", ""),
    ("", "", "BY77 PJCB 3012 0097 9910 0000 0933"),
    # Legal forms at both ends, in both scripts, and two in a row.
    ("РОМАШКА ООО", "", ""),
    ("ЧУП ВАСИЛЁК", "", ""),
    ("ООО ЗАО РОМАШКА", "", ""),
    # The case the reference's ordering exists for: ТОВ is written before
    # ТОВАРИЩЕСТВО..., so a naive port truncates the longer form.
    ("ТОВАРИЩЕСТВО С ОГРАНИЧЕННОЙ ОТВЕТСТВЕННОСТЬЮ АЛМАТЫ", "", ""),
    ("ИНДИВИДУАЛЬНЫЙ ПРЕДПРИНИМАТЕЛЬ ИВАНОВ", "", ""),
    # A form that is only a prefix of the name must survive.
    ("ABBOTT", "", ""),
    ("SAMSUNG", "", ""),
    ("LIMITEDGOODS", "", ""),
    # Polish forms, including the spaced and dotted spellings.
    ("ALLEGRO SP. Z O.O.", "", ""),
    ("ALLEGRO SPZOO", "", ""),
    ("ORLEN S.A.", "", ""),
    ("ORLEN SPÓŁKA Z OGRANICZONĄ ODPOWIEDZIALNOŚCIĄ", "", ""),
    ("ACME INC.", "", ""),
    ("ACME INC", "", ""),
    ("ACME CORP", "", ""),
    # Addresses in the name field, which PKO BP does.
    ("ACME LTD ADDRESS WARSZAWA", "", ""),
    ("ACME LTD ARDRESS WARSZAWA", "", ""),
    ("ACME LTD, WARSZAWA", "", ""),
    ("ACME LTD UL. KOMSOMOLSKAYA 12", "", ""),
    ("ACME LTD 12 WARSZAWA", "", ""),
    ("ACME ROAD RUNNER", "", ""),
    # The first token is never cut: a name may begin with a digit.
    ("1SERVICE", "", ""),
    ("12 ACME", "", ""),
    # Quotes, punctuation and whitespace that must all collapse the same way.
    ('ООО "РОМАШКА"', "", ""),
    ("ООО «РОМАШКА»", "", ""),
    ("ООО  РОМАШКА\t", "", ""),
    ("ООО", "", "BY77PJCB301"),
    # A name that normalises away entirely falls through to the account.
    ("...", "", "BY77PJCB301"),
    ("...", "", ""),
]


# Rows the Belarusian fixtures cannot contain. Each exists to exercise a path
# that BY statements never reach: a regulated code, a bank's own operation
# type, and the layer order between them.
#
# (description, name, tax_id, account, direction, regulated_code, source_kind)
SYNTHETIC_TXNS = [
    # L0.5 from a Kazakh КНП, with a description that a Belarusian text rule
    # also matches. The КНП must win -- that is the whole layer order, and
    # priority alone would give it to the BY rule, which is numbered first.
    ("ЗАРАБОТНАЯ ПЛАТА ЗА МАЙ", "ТОО АЛМАТЫ", "", "", "Expense", "332", "bank"),
    # The same КНП against the other direction, which no rule covers.
    ("ЗАРАБОТНАЯ ПЛАТА ЗА МАЙ", "ТОО АЛМАТЫ", "", "", "Income", "332", "bank"),
    # A Kazakh code with no rule at all falls through to the text layer.
    ("ЗАРАБОТНАЯ ПЛАТА ЗА МАЙ", "ТОО АЛМАТЫ", "", "", "Expense", "999", "bank"),
    # PKO BP's own operation type, which is L0.5 for the same reason.
    ("OPLATA", "ALLEGRO SP. Z O.O.", "", "", "Expense", "Prowizja", "bank"),
    # The specificity pair from task 6.9: both rules match, the longer phrase
    # is numbered first, and the answer must be the longer one's category.
    ("ПОДОХОДНЫЙ НАЛОГ;ИЗ ДИВИДЕНДОВ", "", "", "", "Expense", "", "bank"),
    ("ПОДОХОДНЫЙ НАЛОГ ЗА МАЙ", "", "", "", "Expense", "", "bank"),
    # Nothing matches: the row goes to review, and that is an answer.
    ("СОВЕРШЕННО НЕИЗВЕСТНОЕ НАЗНАЧЕНИЕ", "", "", "", "Expense", "", "bank"),
]

# The organisation's own memory, as the review queue would have filled it.
# Both keys belong to counterparties whose rows a text rule already answers,
# and both point somewhere else on purpose: that is what makes the fixture pin
# L0 *above* L1 rather than merely alongside it. One key is a tax identifier
# and one is a name, so the tier reported as evidence is exercised too.
MEMORY = {
    "tax:120970530": "0401020208",
    "name:ЯНТАРЬЛОГИСТИК": "0401040101",
}


def engine_cases(rows: list[dict[str, str]]) -> list[dict]:
    """Every fixture row, plus the synthetic ones, through `engine.classify`."""
    txns = [
        {
            "purp": r["description"],
            "cp": r["name"],
            "tax": r["tax_id"],
            "acct": r["account"],
            "dir": r["direction"],
            "knp": "",
            "typ": "",
            "src": "bank",
        }
        for r in rows
    ]
    txns += [
        {
            "purp": purp,
            "cp": cp,
            "tax": tax,
            "acct": acct,
            "dir": direction,
            "knp": code,
            "typ": "",
            "src": src,
        }
        for purp, cp, tax, acct, direction, code, src in SYNTHETIC_TXNS
    ]

    rules = json.loads(RULES.read_text(encoding="utf-8"))["rules"]
    cases = []
    for i, t in enumerate(txns):
        # Measured without memory always, and with it only where it could
        # change the answer. The same row answered by L0 in one case and by L1
        # in the other is the clearest statement of the layer order a fixture
        # can make; emitting the pair for every other row would double the
        # file to say nothing.
        key = counterparty_key(t["cp"], t["tax"], t["acct"])[0]
        variants = [("no_memory", None)]
        if key in MEMORY:
            variants.append(("memory", MEMORY))
        for label, memory in variants:
            answer = classify(t, rules, memory)
            cases.append(
                {
                    "id": f"{i}-{label}",
                    "txn": {
                        "description_norm": normalize_description(t["purp"]),
                        "counterparty_key": counterparty_key(
                            t["cp"], t["tax"], t["acct"]
                        )[0],
                        "direction": t["dir"],
                        "regulated_code": t["knp"] or t["typ"],
                        "source_kind": t["src"],
                    },
                    "memory": memory or {},
                    "expected": None
                    if answer is None
                    else {
                        "category_code": answer["category"],
                        "engine_layer": answer["layer"],
                        "confidence": answer["confidence"],
                        "evidence": answer["evidence"],
                        "matched_rule_priority": (
                            0 if answer["layer"] == "L0" else answer["rule"]
                        ),
                    },
                }
            )
    return cases


def main() -> None:
    rows = redacted_rows()
    if not rows:
        raise SystemExit(f"no rows read from {FIXTURES} -- the fixtures moved")

    seen: set[str] = set()
    descriptions = []
    for s in [r["description"] for r in rows] + EXTRA_DESCRIPTIONS:
        if s in seen:
            continue
        seen.add(s)
        descriptions.append({"in": s, "out": normalize_description(s)})

    seen_keys: set[tuple[str, str, str]] = set()
    keys = []
    for name, tax_id, account in [
        (r["name"], r["tax_id"], r["account"]) for r in rows
    ] + EXTRA_KEYS:
        if (name, tax_id, account) in seen_keys:
            continue
        seen_keys.add((name, tax_id, account))
        key, tier = counterparty_key(name, tax_id, account)
        keys.append(
            {
                "name": name,
                "tax_id": tax_id,
                "account": account,
                "key": key,
                "tier": tier,
            }
        )

    OUT.write_text(
        json.dumps(
            {
                "normalize_version": NORMALIZE_VERSION,
                "source": "core/testdata/priorbank-by plus eval/conformance.py",
                "descriptions": descriptions,
                "keys": keys,
            },
            ensure_ascii=False,
            indent=1,
        )
        + "\n",
        encoding="utf-8",
    )
    print(f"{OUT}: {len(descriptions)} descriptions, {len(keys)} counterparty keys")

    cases = engine_cases(rows)
    answered = sum(1 for c in cases if c["expected"] is not None)
    ENGINE_OUT.write_text(
        json.dumps(
            {
                "engine_version": ENGINE_VERSION,
                "normalize_version": NORMALIZE_VERSION,
                "rules": RULES.name,
                "cases": cases,
            },
            ensure_ascii=False,
            indent=1,
        )
        + "\n",
        encoding="utf-8",
    )
    print(f"{ENGINE_OUT}: {len(cases)} cases, {answered} of them answered")


if __name__ == "__main__":
    main()
