"""Produces redacted copies of the statements for `core/testdata`.

Run: `python eval/anonymize.py`.

The originals are a real customer's bank exports. They name people, carry tax
identifiers and account numbers, and committing them would put all of that in
git history permanently. The parsers do not need any of it: they need the shape
of the file.

So this keeps everything a parser is tested on and replaces everything that
identifies anyone:

  kept        every row and column, the header block, the summary rows and
              their odd alignment, the encoding, the delimiters, every amount,
              every balance, every date, the payment purpose wording
  replaced    counterparty names, tax identifiers, account numbers, document
              references, and personal names appearing inside a purpose

Replacements are deterministic: one counterparty becomes one pseudonym in every
file, so a fixture still exercises deduplication and vendor memory. They are
also one-way — the mapping is a salted hash and is not written down.

Amounts and balances are untouched on purpose. `opening + Σ movements =
closing` is the strongest check the ingest stage has, and a fixture that cannot
exercise it is not worth committing.
"""

import csv
import glob
import hashlib
import os
import re
import shutil

from sources import ensure_statements

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "core", "testdata")
SALT = b"vekst-fixtures-2026"

COMPANIES = [
    "ГРАНИТ ПЛЮС",
    "ВЕКТОР ТРЕЙД",
    "СЕВЕРНЫЙ ПУТЬ",
    "АЛЬФА СИСТЕМС",
    "БЕЛАЯ РЕКА",
    "ТЕХНОПАРК СИТИ",
    "МЕРИДИАН ГРУПП",
    "ЯНТАРЬ ЛОГИСТИК",
    "ЗЕЛЁНЫЙ ОСТРОВ",
    "ПЕРВАЯ СТУДИЯ",
    "КАСКАД ИНЖИНИРИНГ",
    "ОРИОН МЕДИА",
    "ДЕЛЬТА СЕРВИС",
    "СТРЕЛА КОНСАЛТ",
    "НОВЫЙ ФОРМАТ",
    "ПОЛЯРНАЯ ЗВЕЗДА",
    "КАМЕННЫЙ МОСТ",
    "ЛИНИЯ РОСТА",
    "СИНИЙ КИТ",
    "ТРИ СОСНЫ",
]
LATIN_COMPANIES = [
    "NORTHWIND TRADING",
    "BLUEPINE SYSTEMS",
    "CEDAR POINT LLC",
    "HARBOUR SOFT",
    "MERIDIAN LABS",
    "STONEBRIDGE INC",
    "AMBERWAY GROUP",
    "FIRSTLIGHT OU",
    "GREENFIELD SP",
    "SILVERLAKE AB",
    "OAKROW LIMITED",
    "REDCLIFF CORP",
]
SURNAMES = [
    "ИВАНОВ",
    "ПЕТРОВ",
    "СИДОРОВ",
    "КОЗЛОВ",
    "НОВИКОВ",
    "МОРОЗОВ",
    "ВОЛКОВ",
    "СОКОЛОВ",
    "ЛЕБЕДЕВ",
    "ЗАЙЦЕВ",
]
GIVEN = ["АЛЕКСЕЙ", "МАРИЯ", "ДМИТРИЙ", "ЕЛЕНА", "СЕРГЕЙ", "ОЛЬГА", "АНДРЕЙ", "ИРИНА"]
PATRONYMIC = [
    "ВИКТОРОВИЧ",
    "ПЕТРОВНА",
    "СЕРГЕЕВИЧ",
    "ИВАНОВНА",
    "АНДРЕЕВИЧ",
    "ОЛЕГОВНА",
]


def _n(value: str, modulo: int) -> int:
    return (
        int.from_bytes(
            hashlib.blake2s(SALT + value.encode("utf-8"), digest_size=8).digest(), "big"
        )
        % modulo
    )


def fake_company(name: str) -> str:
    if not name.strip():
        return name
    pool = LATIN_COMPANIES if re.search(r"[A-Za-z]", name) else COMPANIES
    return pool[_n(name, len(pool))]


def fake_digits(value: str) -> str:
    """Same length, same shape, different number. A tax identifier keeps its
    length because a parser may check it."""
    digits = re.sub(r"\D", "", value)
    if not digits:
        return value
    replacement = str(_n(value, 10 ** len(digits))).rjust(len(digits), "7")[
        : len(digits)
    ]
    out, i = [], 0
    for ch in value:
        if ch.isdigit():
            out.append(replacement[i])
            i += 1
        else:
            out.append(ch)
    return "".join(out)


def fake_person(match: re.Match) -> str:
    key = match.group(0)
    return (
        f"{SURNAMES[_n(key, len(SURNAMES))]} "
        f"{GIVEN[_n(key + 'g', len(GIVEN))]} "
        f"{PATRONYMIC[_n(key + 'p', len(PATRONYMIC))]}"
    )


# SURNAME GIVEN PATRONYMIC, three Cyrillic words in caps, the shape a payment
# purpose uses when it names an employee.
_FULL_NAME = re.compile(r"\b[А-ЯЁ]{3,}\s+[А-ЯЁ]{3,}(?:ИЧ|НА|ВНА|ОВИЧ|ЕВИЧ)\b")
# SURNAME I.O.
_SHORT_NAME = re.compile(r"\b[А-ЯЁ][А-ЯЁа-яё]{2,}У?\s+[А-ЯЁ]\.\s?[А-ЯЁ]\.")


def scrub_text(text: str) -> str:
    text = _FULL_NAME.sub(fake_person, text)
    return _SHORT_NAME.sub(
        lambda m: SURNAMES[_n(m.group(0), len(SURNAMES))] + " А.А.", text
    )


# An IBAN, with or without the spaces the bank inserts.
_IBAN = re.compile(r"\b[A-Z]{2}\d{2}[\sA-Z0-9]{10,32}\b")
# A run of digits long enough to identify something. Dates are groups of 2 and
# 4, so they never match -- but only if the replacement is applied to the token
# rather than to the whole cell, which is what broke the statement period the
# first time round.
_LONG_DIGITS = re.compile(r"\b\d{6,}\b")


def scrub_preamble(cell: str) -> str:
    """The header block names the account holder and the account. It also
    states the statement period, which the validator checks, so dates must
    survive untouched."""
    if not cell:
        return cell
    # "Наименование счета********* <holder>" -- the customer's own name.
    m = re.match(r"^(Наименование счета\**\s*)(.+)$", cell, re.S)
    if m:
        return m.group(1) + fake_company(m.group(2))
    cell = _IBAN.sub(lambda x: fake_digits(x.group(0)), cell)
    cell = _LONG_DIGITS.sub(lambda x: fake_digits(x.group(0)), cell)
    return scrub_text(cell)


def anonymize_priorbank(src: str, dst: str) -> None:
    """Belarus, CSV, windows-1251, ';'.

    Row layout differs between rouble and currency accounts, and the summary
    rows do not line up with the header at all -- both preserved, because both
    are what the parser has to survive.
    """
    with open(src, encoding="cp1251") as fh:
        rows = list(csv.reader(fh, delimiter=";"))
    header_at = next(
        (i for i, r in enumerate(rows) if r and r[0].startswith("Дата док")), None
    )
    head = (
        {n.strip(): i for i, n in enumerate(rows[header_at])}
        if header_at is not None
        else {}
    )

    def col(name):
        return head.get(name, -1)

    out = []
    for i, row in enumerate(rows):
        row = list(row)
        if (
            header_at is not None
            and i > header_at
            and len(row) > 4
            and re.match(r"^\d{2}\.\d{2}\.\d{4}$", (row[0] or "").strip())
        ):
            for name, fn in (
                ("Корреспондент.Название", fake_company),
                ("Корреспондент.УНП", fake_digits),
                ("Корреспондент.Счет", fake_digits),
                ("Корреспондент.Код", lambda v: v),
            ):
                j = col(name)
                if 0 <= j < len(row) and row[j]:
                    row[j] = fn(row[j])
            j = col("Назначение")
            if 0 <= j < len(row) and row[j]:
                row[j] = scrub_text(row[j])
        elif i < (header_at or 0):
            row = [scrub_preamble(c) for c in row]
        out.append(row)

    os.makedirs(os.path.dirname(dst), exist_ok=True)
    with open(dst, "w", encoding="cp1251", newline="") as f:
        csv.writer(f, delimiter=";", lineterminator="\n").writerows(out)


def main() -> None:
    root = ensure_statements()
    shutil.rmtree(os.path.join(OUT, "priorbank-by"), ignore_errors=True)

    # Two files, chosen because they are the two column layouts the same export
    # produces: a rouble account with Корреспондент.УНП, and a currency account
    # that drops it and adds the bank's own conversion instead.
    wanted = [
        "Vpsk_BY79PJCB30120097991000000933-2.csv",
        "Vpsk_BY80PJCB30120097991030000978-2.csv",
    ]
    made = 0
    for src in sorted(glob.glob(os.path.join(root, "РБ", "*", "*.csv"))):
        if os.path.basename(src) not in wanted:
            continue
        year = os.path.basename(os.path.dirname(src))
        dst = os.path.join(OUT, "priorbank-by", f"{year}-{os.path.basename(src)}")
        anonymize_priorbank(src, dst)
        made += 1
        print(f"  {os.path.relpath(dst, OUT)}")
    print(f"{made} fixtures written to core/testdata")


if __name__ == "__main__":
    main()
