"""Readers for the founder-supplied source files.

Everything here reads `$VEKST_DOCCL` (default `../docCl`, a sibling of the
repository). Those files are the customer's own exports and their accountant's
categorisation; they are deliberately not committed.

Three countries, three banks, three formats — and the differences are the
point, so each reader states what its format does that the others do not.
"""

import csv
import glob
import os
import re
import shutil
import zipfile

import openpyxl
import xlrd

_HERE = os.path.dirname(os.path.abspath(__file__))
DOCCL = os.environ.get("VEKST_DOCCL", os.path.join(_HERE, "..", "..", "docCl"))

# The statements arrive as three zips. They are unpacked once into a cache
# beside this file rather than read in place, so nothing writes into the
# founder's folder. The cache is gitignored: it holds customer data.
CACHE = os.environ.get("VEKST_STATEMENTS", os.path.join(_HERE, ".statements"))


def _zip_name(info: zipfile.ZipInfo) -> str:
    """Recover the real filename.

    These archives were made on macOS and store UTF-8 names without setting the
    flag that says so, so `zipfile` falls back to CP437 and "РБ" arrives as
    "╨á╨æ". Undo that. Directory names matter here: the loaders find statements
    by country folder.
    """
    if info.flag_bits & 0x800:
        return info.filename
    try:
        return info.filename.encode("cp437").decode("utf-8")
    except (UnicodeDecodeError, UnicodeEncodeError):
        return info.filename


def ensure_statements() -> str:
    """Unpack the statement archives on first use. Idempotent."""
    if os.path.isdir(CACHE) and glob.glob(os.path.join(CACHE, "*", "**", "*.*"), recursive=True):
        return CACHE
    os.makedirs(CACHE, exist_ok=True)
    for z in glob.glob(os.path.join(DOCCL, "*.zip")):
        with zipfile.ZipFile(z) as zf:
            for info in zf.infolist():
                name = _zip_name(info)
                if name.startswith("__MACOSX/") or name.endswith("/"):
                    continue
                target = os.path.join(CACHE, *name.split("/"))
                os.makedirs(os.path.dirname(target), exist_ok=True)
                with zf.open(info) as src, open(target, "wb") as dst:
                    shutil.copyfileobj(src, dst)
    return CACHE


def _find(fragment: str) -> str:
    """macOS writes filenames decomposed, so matching on a Cyrillic prefix
    fails. Match on the ASCII fragment every one of these names carries."""
    hits = [p for p in glob.glob(os.path.join(DOCCL, "*.xlsx")) if fragment in p]
    if not hits:
        raise FileNotFoundError(f"{fragment} not found under {DOCCL}")
    return hits[0]


# --------------------------------------------------------------------------
# The accountant's categorisation tables: 590 hand-written rules, three files.
# --------------------------------------------------------------------------

def load_rules(country: str) -> list[dict]:
    """One rule per row: (purpose contains) AND (counterparty contains) AND
    (direction) -> a five-level category path."""
    path = _find({"BY": "_BY_PL", "KZ": "_KZ_PL", "PL": "_PO_PL"}[country])
    rows = list(openpyxl.load_workbook(path, data_only=True)["PL"].iter_rows(values_only=True))
    head = {str(v).strip(): i for i, v in enumerate(rows[0]) if v}

    def cell(row, key):
        i = head.get(key)
        return str(row[i]).strip() if i is not None and row[i] is not None else ""

    out = []
    for row in rows[1:]:
        if not any(v not in (None, "") for v in row):
            continue
        path = [cell(row, f"PL{i}") for i in range(1, 6)]
        # The Polish file ends with two rows that carry a stray cell but no
        # matcher and no category. Left in, they match every transaction and
        # assign nothing, so everything below them looks unclassified — which
        # is how 85% of card payments came to read as unlabelled.
        if not any(p for p in path):
            continue
        out.append({
            # Poland names the matched field differently; it holds the same thing.
            "purp": cell(row, "Назначение") or cell(row, "Dane operacji"),
            "cp": cell(row, "Корреспондент.Название"),
            "dir": cell(row, "Income/Expense"),
            "path": path,
        })
    return out


# --------------------------------------------------------------------------
# Statements.
# --------------------------------------------------------------------------

def _num(s) -> float:
    if isinstance(s, (int, float)):
        return float(s or 0)
    return float((str(s or "0").replace("\xa0", " ").replace(" ", "").replace(",", ".")) or 0)


def by_txns() -> list[dict]:
    """Priorbank, Belarus. CSV, windows-1251, ';'.

    Two column layouts in one export: rouble accounts carry
    `Корреспондент.УНП`, currency accounts drop it and add
    `Эквивалент.*` — the bank's own conversion to the base currency. So the
    column map is read from the header row, never fixed per bank.
    """
    out = []
    for f in sorted(glob.glob(os.path.join(ensure_statements(), "РБ", "*", "*.csv"))):
        rows = list(csv.reader(open(f, encoding="cp1251"), delimiter=";"))
        hi = next((i for i, r in enumerate(rows) if r and r[0].startswith("Дата док")), None)
        if hi is None:
            continue
        head = {n.strip(): i for i, n in enumerate(rows[hi])}
        get = lambda r, k: (r[head[k]] if k in head and head[k] < len(r) else "") or ""
        for r in rows[hi + 1:]:
            if len(r) < 5 or not re.match(r"^\d{2}\.\d{2}\.\d{4}$", (r[0] or "").strip()):
                continue
            debit, credit = _num(get(r, "Номинал.Дебет")), _num(get(r, "Номинал.Кредит"))
            out.append({
                "cp": get(r, "Корреспондент.Название"),
                "tax": get(r, "Корреспондент.УНП"),
                "acct": get(r, "Корреспондент.Счет"),
                "purp": get(r, "Назначение"),
                "knp": "",
                "dir": "Expense" if debit > 0 else "Income",
                "amt": max(debit, credit),
            })
    return out


def kz_txns() -> list[dict]:
    """Kazakhstan. .xls, 40 columns, statement totals repeated on every row.

    Column 16 is КНП — the state payment-purpose code, present on every row.
    It is regulator-issued, which makes it a stronger signal than any text
    match, and it arrives on a *bank* row rather than a ledger one.
    """
    out = []
    for f in sorted(glob.glob(os.path.join(ensure_statements(), "КЗ", "*", "*.xls"))):
        sh = xlrd.open_workbook(f).sheet_by_index(0)
        for i in range(1, sh.nrows):
            v = lambda j: str(sh.cell_value(i, j)).strip()
            out.append({
                "cp": v(19), "tax": v(20), "acct": v(21),
                "purp": v(17), "knp": v(16),
                "dir": "Expense" if v(3) == "DEBIT" else "Income",
                "amt": max(_num(sh.cell_value(i, 10)), _num(sh.cell_value(i, 12))),
            })
    return out


PL_KEYS = [
    "Rachunek kontrahenta", "Nazwa i adres Kontrahenta", "Tytuł", "Rachunek",
    "Lokalizacja", "Numer karty", "Numer referencyjny",
    "Oryginalna kwota operacji", "Identyfikator transakcji", "Data i czas operacji",
]
_PL_FIELD = re.compile(r"\s*(" + "|".join(map(re.escape, PL_KEYS)) + r")\s*:\s*(.*)$", re.S)


def pl_parse(s: str) -> dict:
    """`Dane operacji` is a pipe-separated key:value structure, not free text.
    The counterparty is in there — it just needs unpacking."""
    out = {}
    for part in (s or "").split("|"):
        m = _PL_FIELD.match(part)
        if m:
            out[m.group(1)] = m.group(2).strip()
    return out


def pl_txns() -> list[dict]:
    """PKO BP, Poland. .xls, four sheets, one signed amount column and three
    currencies in one file."""
    out = []
    for f in sorted(glob.glob(os.path.join(ensure_statements(), "ПЛ", "*.xls"))):
        sh = xlrd.open_workbook(f).sheet_by_name("Dane")
        for i in range(1, sh.nrows):
            d = pl_parse(str(sh.cell_value(i, 2)))
            loc = d.get("Lokalizacja", "")
            # Card payments name the merchant after a '*': "DNH*GODADDY.COM".
            merchant = loc.split("*")[-1].strip() if "*" in loc else ""
            amt = sh.cell_value(i, 4) or 0
            out.append({
                "cp": d.get("Nazwa i adres Kontrahenta", "") or merchant,
                "tax": "", "acct": d.get("Rachunek kontrahenta", ""),
                "purp": d.get("Tytuł", ""), "knp": "",
                # The bank's own operation type. Not regulated the way КНП is,
                # but it separates a fee from a transfer from a card payment
                # without reading any text, and it is the only field here that
                # classifies anything on its own.
                "typ": str(sh.cell_value(i, 3)).strip(),
                "dir": "Income" if amt > 0 else "Expense",
                "amt": abs(amt),
            })
    return out


STATEMENTS = {"BY": by_txns, "KZ": kz_txns, "PL": pl_txns}
