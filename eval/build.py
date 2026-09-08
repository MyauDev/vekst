"""Builds the taxonomy and the country rule templates from the source files.

Run: `python eval/build.py`. Writes everything to `eval/out/`.

Two products come out of here:

  * **The taxonomy** — one category tree merged from the P&L skeleton and the
    real leaf names the accountant used. This is decision D-1 in
    `docs/IMPLEMENTATION_PLAN.md`, and changes 3.1 and 3.2 are blocked without it.

  * **The templates** — rules that belong to a country and a bank, not to a
    customer. They are the reusable half: a new Belarusian company on Priorbank
    is 87% classified before it configures anything.

Every decision taken while merging is named below, next to the code that
applies it, so a later reader can disagree with a specific line rather than
with the file.
"""

import csv
import json
import os
import re
from collections import Counter, defaultdict

from norm import NORMALIZE_VERSION, normalize_description
from sources import load_rules

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "out")
TAXONOMY_VERSION = "v1"

# --------------------------------------------------------------------------
# Canonicalisation. The three files spell the same category several ways.
# --------------------------------------------------------------------------

L1_CANON = {
    "OTHER In&Ex": "OIE",
    "Financial Result": "FR",          # "FR" and "Financial Result" are one section
    "CIT EXPENSE": "CIT",
    "OUT of P&L": "OUT OF P&L",        # case only
}

PATH_FIX = {
    ("CIT", "CIT expense", "CIT expense"): ("CIT",),
    ("OIE", "OTHER EXPENSE", "Other taxes", "IT Park Membeship"):
        ("OIE", "OTHER EXPENSE", "Other taxes", "IT Park - membership"),
    # Used both as a leaf and as a section. A node cannot be both.
    ("OIE", "OTHER EXPENSE", "Other taxes"):
        ("OIE", "OTHER EXPENSE", "Other taxes", "Other"),
    # "Dev Services" under Marketing is a copy-paste: its siblings live under
    # Production > Developers, and 8 of the 11 rules put them there.
    ("OPEX", "Marketing and Sales", "Marketing", "Dev Services", "Design Tools"):
        ("OPEX", "Production", "Developers", "Dev Services", "Design Tools"),
    ("OPEX", "Marketing and Sales", "Marketing", "Dev Services", "Production Tools"):
        ("OPEX", "Production", "Developers", "Dev Services", "Production Tools"),
    # One rule gave SEO its own level; the other two nest it under MRK Services.
    ("OPEX", "Marketing and Sales", "Marketing", "SEO", "SEO Tools"):
        ("OPEX", "Marketing and Sales", "Marketing", "MRK Services", "SEO"),
    # Same leaf in two places. Tools for selling belong to Sales.
    ("OPEX", "Marketing and Sales", "Marketing", "MRK Services", "Sales Tools"):
        ("OPEX", "Marketing and Sales", "Sales", "SL Services", "Sales Tools"),
    # Bookkeeping is bought in, so it is a Service, not an Expense.
    ("OPEX", "Administration", "Finance", "FI Expenses", "Acc. matters"):
        ("OPEX", "Administration", "Finance", "FI Services", "Acc. matters"),
    ("OPEX", "Administration", "Legal", "Leg. Services", "NN"):
        ("OPEX", "Administration", "Legal", "Leg. Services", "Legal - other"),
}

NODE_RENAME = {"Recruting": "Recruiting"}                    # typo, in both sources
# "GM" is Gross Margin at the top level and General Management here. One
# abbreviation, two meanings, both on the same report.
NODE_RENAME_AT = {("OPEX", "Administration", "GM"): "General Management"}

# Classification says "this is payroll"; splitting it across departments needs
# to know which employee, which no template can. Kept as its own state.
REQUIRES_ALLOCATION = {("OPEX", "PAYROLL to distribute"), ("OPEX", "PAYROLL TAX to distribute")}
# A vendor name sitting at level 2, where the default scope is global.
ORG_SCOPED = {("OPEX", "Twoj StartUp")}
NON_PNL = {"CAPEX", "OUT OF P&L"}
L1_ORDER = ["NET SALES", "CS", "OCS", "OPEX", "OIE", "FR", "CIT", "CAPEX", "OUT OF P&L"]
COMPUTED = [("91", "GM", "NET SALES - CS"), ("92", "NM", "GM - OCS"),
            ("93", "CM", "NM - OPEX - OIE"), ("94", "IBT", "CM - FR"),
            ("95", "NI", "IBT - CIT")]


def canon(path) -> tuple:
    p = tuple(x.strip() for x in path if x and str(x).strip())
    if not p:
        return p
    p = (L1_CANON.get(p[0], p[0]),) + p[1:]
    p = PATH_FIX.get(p, p)
    return tuple(NODE_RENAME.get(x, x) for x in p)


def build_taxonomy():
    """Merge the three rule files into one tree, and pull customers out of it."""
    used, customers = Counter(), Counter()
    for country in ("BY", "KZ", "PL"):
        for r in load_rules(country):
            p = canon(r["path"])
            if not p:
                continue
            # 84 of the 158 paths were customer names under one branch. A
            # customer is a dimension of revenue, not a kind of it: keeping them
            # as categories grows the taxonomy with every sale and makes it
            # per-organisation in its entirety.
            if p[:2] == ("NET SALES", "SOFTWARE DEVELOPMENT") and len(p) == 3:
                if p[2] == "Return of payment":
                    p = ("OIE", "OTHER INCOME", "Return of payment")
                else:
                    customers[p[2]] += 1
                    p = ("NET SALES", "SOFTWARE DEVELOPMENT")
            used[p] += 1

    nodes = defaultdict(int)
    for path, n in used.items():
        for i in range(1, len(path) + 1):
            nodes[path[:i]] += n
    # Zero rules in two years, but the GM formula subtracts it, so the line exists.
    nodes.setdefault(("CS",), 0)

    order = sorted(nodes, key=lambda t: (L1_ORDER.index(t[0]) if t[0] in L1_ORDER else 99, t))
    code, seq = {}, {}
    for t in order:
        parent = t[:-1]
        seq[parent] = seq.get(parent, 0) + 1
        code[t] = (code[parent] + f"{seq[parent]:02d}") if parent else f"{L1_ORDER.index(t[0]) + 1:02d}"
    leaves = {t for t in nodes if not any(o != t and o[: len(t)] == t for o in nodes)}

    cats = {}
    for t in order:
        cats[t] = {
            "code": code[t], "level": len(t),
            # Levels 1-2 are shared and versioned by us; 3+ belong to the
            # organisation. Scope is stored, not derived from depth, so an
            # exception like Twoj StartUp can exist without breaking the rule.
            "scope": "org" if (len(t) >= 3 or t in ORG_SCOPED) else "global",
            "name": NODE_RENAME_AT.get(t, t[-1]),
            "leaf": t in leaves, "is_pnl": t[0] not in NON_PNL,
            "alloc": t in REQUIRES_ALLOCATION, "rules": nodes[t],
        }
    return cats, customers


# --------------------------------------------------------------------------
# Belarus template: text rules that belong to the country or to Priorbank.
# --------------------------------------------------------------------------

# Priorbank's own vocabulary. True for every Priorbank customer, false for a
# Belarusian company banking elsewhere -- hence a separate scope.
BANK_MARKERS = [
    "ПЕРЕОЦЕНКА", "УПЛАТА ПРОЦЕНТОВ ПО ОСТАТКАМ", "ПЕРЕВОД С ПРОДАЖЕЙ ПО БАЗОВОМУ КУРСУ",
    "СВОБОДНАЯ ПРОДАЖА ВАЛЮТЫ", "ЕЖЕМЕСЯЧНАЯ АБОНЕНТСКАЯ ПЛАТА", "ПЛАТА ЗА БЕЗНАЛ",
    "ПЛАТА ЗА ЗАЧИСЛЕНИЕ", "ПЛАТА ЗА УСЛУГИ \"ПРИОРБАНК\"", "ПЕРЕВОД С КОНВЕРСИЕЙ",
]
# Knowledge about this company: its own affiliates and its own people by name.
# Never portable, so never in a template.
CLIENT_MARKERS = [
    "PLAVNO", "ЕФРЕМО", "ЗАЙЦЕВА", "КРОМАН",
    "ОПЛАТА ЗА УСЛУГИ ФАЙЛОВОЕ ПРОСТРАНСТВО", "ОФОРМЛЕНИЕ КАРТЫ ДОСТУПА",
    "ЗА ФОТО", "ЗА ФОР",
]
# Holiday pay is wages, not a tax on wages. Everything else in the tax bucket
# is a contribution to the state; this one is money paid to employees.
CATEGORY_FIX = {"ОТПУСКНЫЕ": ("OPEX", "PAYROLL to distribute")}


def build_by_template(cats):
    scored = []
    for r in load_rules("BY"):
        if r["cp"]:
            continue                       # counterparty rules are L0 memory, not L1
        text = normalize_description(r["purp"])
        if any(m in text for m in CLIENT_MARKERS):
            continue
        path = CATEGORY_FIX.get(text) or canon(r["path"])
        if path not in cats:
            continue
        scope = "bank:priorbank" if any(m in text for m in BANK_MARKERS) else "country:BY"
        scored.append({"country": "BY", "scope": scope, "field": "description",
                       "op": "contains_all", "value": r["purp"], "direction": r["dir"],
                       "path": path})
    # Specific beats general: more parts first, then longer. This is what keeps
    # "ПОДОХОДНЫЙ НАЛОГ;ИЗ ДИВИДЕНДОВ" ahead of "ПОДОХОДНЫЙ НАЛОГ", and
    # "ОТЧИСЛЕНИЯ В ФСЗН" ahead of "ОТПУСКНЫЕ" on a row containing both.
    scored.sort(key=lambda r: (-len([p for p in r["value"].split(";") if p.strip()]),
                               -len(r["value"])))
    return scored


# --------------------------------------------------------------------------
# Kazakhstan template: the state payment-purpose code.
# --------------------------------------------------------------------------
# КНП is filled on 100% of rows and is issued under a regulated classifier, so
# it beats any text match. Note it arrives on a *bank* row -- ARCHITECTURE.md
# restricts the account-code layer to ledger rows, and that restriction is
# wrong for this market.
KNP = {
    ("841", "Expense"): ("OPEX", "Administration", "Finance", "FI Services", "Bank commission"),
    ("851", "Income"):  ("NET SALES", "SOFTWARE DEVELOPMENT"),
    ("859", "Income"):  ("NET SALES", "SOFTWARE DEVELOPMENT"),
    ("857", "Income"):  ("NET SALES", "SOFTWARE DEVELOPMENT"),
    ("858", "Income"):  ("NET SALES", "SOFTWARE DEVELOPMENT"),
    ("223", "Income"):  ("FR", "FXR", "Currency exchange"),
    ("223", "Expense"): ("FR", "FXR", "Currency exchange"),
    ("230", "Expense"): ("FR", "FXR", "Currency exchange"),
    ("911", "Expense"): ("OPEX", "PAYROLL TAX to distribute"),          # ИПН
    ("912", "Expense"): ("OIE", "OTHER EXPENSE", "Tax fines"),          # пеня по ИПН
    ("010", "Expense"): ("OPEX", "PAYROLL TAX to distribute"),          # ОПВ
    ("089", "Expense"): ("OPEX", "PAYROLL TAX to distribute"),          # ОПВ работодателя
    ("012", "Expense"): ("OPEX", "PAYROLL TAX to distribute"),          # соц. отчисления
    ("121", "Expense"): ("OPEX", "PAYROLL TAX to distribute"),          # отчисления ОСМС
    ("122", "Expense"): ("OPEX", "PAYROLL TAX to distribute"),          # взносы ОСМС
    ("332", "Expense"): ("OPEX", "PAYROLL to distribute"),              # зарплата, отдел неизвестен
    ("661", "Expense"): ("OUT OF P&L",),                                # дивиденды
    ("120", "Expense"): ("OIE", "OTHER EXPENSE", "Other taxes", "Other"),
    # The path keeps the source spelling "GM"; NODE_RENAME_AT only changes the
    # label a human sees, so the tree keeps one identity for a node.
    ("852", "Expense"): ("OPEX", "Administration", "GM", "GM Services", "Communication"),
    ("855", "Expense"): ("OPEX", "Administration", "Finance", "FI Services", "1С Cloud"),
    ("854", "Expense"): ("OPEX", "Administration", "Legal", "Leg. Services", "Claim"),
}


def build_kz_template(cats):
    out = []
    for (code, direction), path in sorted(KNP.items()):
        if path not in cats:
            raise KeyError(f"КНП {code} points at a category that does not exist: {path}")
        out.append({"country": "KZ", "scope": "country:KZ", "field": "knp", "op": "eq",
                    "value": code, "direction": direction, "path": path})
    return out
