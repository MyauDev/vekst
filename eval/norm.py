"""Normalisation. Two pure functions, no state, no clock, no database.

Both are versioned, because changing either changes what counts as a match:
`normalize_description` changes which rules fire, and `counterparty_key`
changes what counts as the same counterparty. A change to either is a backfill
of every row that carries the version, never a silent reinterpretation.

Ported to Go or kept in Python for the classifier — either way the fixtures in
`out/` are the contract, and `run_eval.py` is what proves a port is faithful.
"""

import re
import unicodedata

NORMALIZE_VERSION = "v1"

# Bank boilerplate. Carries no meaning and breaks otherwise-identical matches.
# Every entry here was found in a real statement, not imagined:
#   "Частичная оплата."     Priorbank, when it takes a fee in instalments
#   "Rachunek kontrahenta:" PKO BP, followed by the counterparty account
#   "/ROC/" "/PURP/"        SWIFT tags on cross-border payments
NOISE_PREFIX = [
    r"ЧАСТИЧНАЯ ОПЛАТА\.",
    r"RACHUNEK KONTRAHENTA:",
    r"TYTUL:",
    r"TYTUŁ:",
    r"/ROC/",
    r"/PURP/",
    r"/URI/",
]

QUOTES = str.maketrans(
    {"«": '"', "»": '"', "“": '"', "”": '"', "„": '"', "‘": "'", "’": "'", " ": " "}
)


def normalize_description(s: str) -> str:
    """Canonical form of a payment purpose, for matching rules against it.

    Both the rule's stored value and the transaction's text go through this,
    so a bank prefix present on one side and absent on the other stops
    mattering.
    """
    if not s:
        return ""
    # NFKC first: files exported on macOS arrive decomposed, so "й" is two
    # code points there and one here. Without this they never compare equal.
    s = unicodedata.normalize("NFKC", s).translate(QUOTES).upper()
    for p in NOISE_PREFIX:
        s = re.sub(p, " ", s)
    # "03 5954 0014 67" -> "035954001467". A Polish counterparty account is
    # written three ways in one export; without this each spelling needs its
    # own rule, and 18 of them did.
    s = re.sub(r"(?<=\d)[\s\-]+(?=\d)", "", s)
    return re.sub(r"\s+", " ", s).strip()


LEGAL_FORMS = [
    r"ООО", r"ОДО", r"ЗАО", r"ОАО", r"УП", r"ЧУП", r"ИП", r"ТОО", r"АО", r"ТОВ", r"ПАО",
    r"ИНДИВИДУАЛЬНЫЙ ПРЕДПРИНИМАТЕЛЬ",
    r"ТОВАРИЩЕСТВО С ОГРАНИЧЕННОЙ ОТВЕТСТВЕННОСТЬЮ",
    r"LLC", r"LLP", r"LTD", r"LIMITED", r"INC\.?", r"CORP\.?", r"GMBH", r"AB", r"OU", r"OÜ",
    r"SARL", r"SRL", r"PTY", r"BV", r"NV", r"SA", r"AG", r"KG",
    r"SPOLKA Z OGRANICZONA ODPOWIEDZIALNOSCIA",
    r"SPÓŁKA Z OGRANICZONĄ ODPOWIEDZIALNOŚCIĄ",
    r"SP\.?\s?Z\.?\s?O\.?\s?O\.?", r"S\.A\.",
]
_LF = re.compile(r"(?:^|\s)(?:" + "|".join(LEGAL_FORMS) + r")(?=\s|$)")

# Start-of-address markers. Some banks (PKO BP) return name and address in one
# field, and an address is reformatted far more often than a counterparty
# changes. "ARDRESS" is not a typo here — it is a typo in the source data.
_ADDR = re.compile(
    r"(?:^|\s)(?:ADDRESS|ADRESS|ARDRESS|ADRES|UL\.|STR\.|ST\.|PLAZA|ROAD)\b"
    r"|,"
    r"|(?<=\s)\d{2,}"
)


def strip_address(name: str) -> str:
    """Drop the address, keep the name. The first token is never cut:
    "1SERVICE" is a name, not a house number."""
    if not name:
        return name
    head, _, _ = name.partition(" ")
    rest = name[len(head):]
    m = _ADDR.search(rest)
    return (head + rest[: m.start()]).strip() if m else name.strip()


def counterparty_key(name: str = "", tax_id: str = "", account: str = "") -> tuple[str, str]:
    """Stable identity of a counterparty, and the tier that produced it.

    The tier matters downstream: a tax identifier is regulator-issued and
    exact, a name is a guess that survived normalisation, and an account is a
    last resort. A caller deciding whether to auto-accept should look at it.

    In CIS statements the tax number arrives in its own column — УНП in
    Belarus, БИН/ИИН in Kazakhstan — which is why it is the first tier rather
    than something parsed out of free text.
    """
    tid = re.sub(r"\D", "", tax_id or "")
    if tid and tid != "0" and len(tid) >= 8:
        return f"tax:{tid}", "tax_id"

    n = unicodedata.normalize("NFKC", strip_address(name or "")).translate(QUOTES).upper()
    n = re.sub(r"[\"'`]", " ", n)
    n = _LF.sub(" ", n)
    n = re.sub(r"[^\w\s]", " ", n, flags=re.UNICODE)
    n = re.sub(r"\s+", "", n)
    if n:
        return f"name:{n}", "name"

    acc = re.sub(r"\W", "", (account or "").upper())
    return (f"acct:{acc}", "account") if acc else ("", "none")
