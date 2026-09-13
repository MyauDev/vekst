> **RETROACTIVE.** Written from `core/internal/ingest` as it stands at `origin/main`
> b4d8dca. Every decision below is one the code already took; this records the reasoning
> where the code carries it, and names the two places the code contradicts the planning
> documents on purpose.

## Context

A parser is the one place in this system that meets a file nobody on the team wrote. Three
properties of real CIS exports decide its shape, and all three were found in actual files
rather than in documentation:

- **The column layout is not fixed per bank.** A Priorbank rouble account carries
  `Корреспондент.УНП`; a currency account drops it and adds `Эквивалент.*`, the bank's own
  conversion. Both arrive in one download.
- **Summary rows do not use the header's columns.** `Обороты` and `Исходящее сальдо` put
  their figures elsewhere entirely.
- **Debit and credit are both always populated,** one of them `0,00`.

Each of those breaks a reasonable-sounding design, and the third breaks a rule written into
two planning documents.

## Goals / Non-Goals

**Goals:** the `Parser` contract, format detection, charset and number-locale handling, the
Priorbank reader, and line-number provenance.

**Non-goals:** validation, persistence, upload, profiles. Named in the proposal.

## The data model

None. This change adds no table and no migration. Its output is a value:

```go
type Statement struct {
    Format   string   // the parser that produced this, recorded on the batch
    Account, Currency, Holder string

    // Declared by the bank, not computed by us.
    Opening, Closing              money.Money
    DeclaredDebit, DeclaredCredit money.Money

    Rows []Row
}

type Row struct {
    LineNo int // 1-based, in the ORIGINAL file

    BookedOn   string // raw; date parsing is validation's, not the parser's
    DocumentNo string
    CounterpartyName, CounterpartyTaxID, CounterpartyAccount string
    Description string

    // КНП, a 1C account number, PKO BP's Typ operacji. Empty for Priorbank.
    RegulatedCode string

    Debit, Credit money.Money
}
```

`money.Money` throughout, never a float and never a string amount. `BookedOn` stays a
string on purpose: a date that fails to parse is a validation error with a line number
attached, and a parser that returns `time.Time` has to decide what to do with a bad date
before anything is in a position to report it.

`RegulatedCode` is present and empty for every Belarusian row. It is carried ahead of need
because a code assigned by someone other than the payer is far stronger evidence than free
text — `eval/README.md` measures sixteen Kazakh КНП pairs classifying with no exceptions —
and adding the field after transactions exist means migrating them. Migration 007's
`regulated_code` column is the other end of it.

## D1 — One parser per bank, behind a registry

**Rejected: one CSV reader with a configuration struct.** It is the obvious shape and it
survives exactly one bank. Priorbank alone needs a seven-line preamble skipped, two column
layouts, and summary rows read positionally rather than by header. A configuration struct
expressive enough for that is a programming language with worse error messages.

**Chosen: a `Parser` interface, registered from each parser's `init`.** Adding a bank is
adding a file. `Formats()` lists what is registered, for the error message and for the
upload screen's "we can read these".

Every implementation is a pure function over bytes — no database handle, no clock, no
network — so a statement can be re-parsed months later to answer a question about a report
that was printed then. This is the same rule `ARCHITECTURE.md` §2.3 sets for the
classification engine, for the same reason.

## D2 — Ambiguous detection is an error, not a first-match win

`ParserFor` collects every parser whose `Detect` returns true.

- None: `ErrUnknownFormat`, carrying the list of formats tried. "Unsupported file" on its
  own tells a customer nothing they can act on.
- One: that parser.
- More than one: an error naming all of them.

The last case is the point. Two parsers claiming one file means one detects too loosely, and
finding that out here — with both names in the message — is cheaper than finding it out from
a customer whose numbers came from the wrong reader.

Detection is correspondingly strict: Priorbank requires the bank's name **and** the header
token `Дата док` within the first 4 KiB. Either alone is too loose, because the word
Приорбанк appears in the payment purpose of every bank-fee row in a statement from anywhere.

## D3 — Charset by scoring, not by declaration

CIS exports are windows-1251 or CP866 and say so nowhere. Valid UTF-8 is a reliable
positive: Cyrillic in windows-1251 is single bytes in 0xC0–0xFF, which break UTF-8's
continuation rules almost immediately, so a file that decodes cleanly as UTF-8 is UTF-8.

Everything else is decided by scoring a sample: +1 per Cyrillic letter, −2 per box-drawing
character, over the first 8 KiB. Box drawing is the wrong page's tell — CP866 and
windows-1251 disagree in exactly that range — and the first kilobytes of a statement are its
header, which is always prose.

`Decode` does **not** treat a resulting U+FFFD as an error. It is silent data corruption and
it belongs to validation, which rejects the batch for it. A parser that failed here would
report "decoding error" where the customer needs "row 2,847, the charset guess was wrong".

## D4 — Numbers never pass through a float

`DecimalFromLocale` strips spaces and non-breaking spaces, then resolves the decimal
separator by position: whichever of `,` and `.` comes last is the decimal point and the
other groups thousands. `"2.009,07"` and `"2,009.07"` are both unambiguous under that rule.

The result is a plain decimal **string** handed to `money.Parse`, which scales it by the
currency's own exponent into `int64` minor units. At no point is there a `float64`. This is
the money invariant holding at the exact boundary where it is most tempting to break it —
`strconv.ParseFloat` is one line and would be wrong by fractions of a cent on a few rows in
every file.

## D5 — The check that two documents get wrong

`IMPLEMENTATION_PLAN.md` §1.1 and `ARCHITECTURE.md` §4a.1 both state a correctness check as:

> Debit and credit are mutually exclusive — fails when **both columns populated** on one row

In every real Priorbank export both columns are populated on every row, one of them `0,00`.
Read literally, that check rejects every row of every file.

The rule that was meant is **both non-zero**. The parser preserves both columns as
`money.Money` and asserts the invariant in
`TestDebitAndCreditAreBothPresentButNeverBothNonZero`; change 2.3 implements the check in
that form. Both documents should be corrected rather than quietly worked around — task 5.2.

## D6 — The balance check is measured here and decided in 2.3

`Statement.BalanceCheck()` computes `opening + Σcredits − Σdebits` against the declared
closing balance and returns `(ok, difference, err)`. Zero tolerance, in `int64` minor units,
which is the only arithmetic in which "zero tolerance" means anything.

It lives here because it needs the parsed rows and the bank's own declared figures together,
and only the parser has both. It returns the difference rather than a verdict because the
verdict — `valid`, `valid_with_warnings`, an override, a line in the error report — is
change 2.3's, and a parser that decided it would be deciding what to tell a customer.

Verified against every fixture in `core/testdata`, both column layouts, by
`TestBalanceReconcilesOnEveryFixture`.

## D7 — Summary rows are read positionally

`Обороты` and `Исходящее сальдо` do not use the header's columns. They are read by taking
the last two numeric cells of the row.

**Rejected: read them by header index.** On a currency account that silently mixes the
account's nominal movements with its rouble-equivalent balances — two different numbers,
both plausible, no error. A wrong balance check is worse than none, because it is believed.

## What this touches from the invariants list

- **Money** — directly, and at the boundary. D4.
- **Validation is blocking and atomic** — respected by omission: this package decides
  nothing. D6.
- **Tenant isolation** — none. No table, no query, no `org_id`. The package never sees a
  database.
- **`source_kind` and the classifier contract** — neither.

## Rejected alternatives

| Rejected | Why |
| --- | --- |
| One configurable CSV reader | A configuration struct expressive enough for Priorbank alone is a programming language with worse error messages. See D1 |
| First-match-wins detection | Two parsers claiming a file means one is too eager; the customer finds out via wrong numbers. See D2 |
| Detect the format from the file extension | Every one of these is `.csv`. The extension says nothing about which bank wrote it |
| Trust a declared charset | CIS exports declare none. See D3 |
| `strconv.ParseFloat` for amounts | One line, and wrong by fractions of a cent on a few rows in every file. See D4 |
| Return `time.Time` from the parser | A bad date then has to be handled before anything can report it with a line number. `BookedOn` stays raw |
| Treat U+FFFD as a parse error | It is a validation outcome with a line number, not a decoding failure. See D3 |
| Read summary rows by header index | Silently mixes nominal movements with rouble-equivalent balances on a currency account. See D7 |
| Implement the balance verdict here | The verdict is what the customer is told, and that is 2.3's. See D6 |

## Risks

| Risk | Mitigation |
| --- | --- |
| A second bank's `Detect` is written loosely and starts claiming Priorbank files | `ParserFor` refuses on ambiguity and names both parsers, so this is a failing test rather than wrong numbers. D2 |
| The charset scorer picks the wrong page on a short or unusual file | A wrong page produces U+FFFD, which validation rejects with a line number rather than importing corrupt text |
| `BalanceCheck` is reimplemented in 2.3 because the seam is not obvious | D6 states it, and 2.3's tasks call this function rather than the arithmetic |
| `raw_rows` is never persisted, so a parse cannot be re-examined without the original file | Task 4.1 is open and belongs with 2.1's batch. The object store keeps the original file, so nothing is unrecoverable meanwhile |
| The corrected debit/credit rule is applied here and the old wording stays in two documents | Task 5.2 corrects both documents rather than leaving the next reader to rediscover it |
