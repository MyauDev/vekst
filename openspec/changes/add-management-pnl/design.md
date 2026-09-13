## Context

The taxonomy already carries the shape of the report. Fourteen level-1 nodes, of which nine
are sections that receive transactions and five are arithmetic over them:

| Code | | |
| --- | --- | --- |
| `01` | NET SALES | |
| `02` | CS | cost of sales |
| `03` | OCS | other cost of sales |
| `04` | OPEX | |
| `05` | OIE | other income and expense |
| `06` | FR | financial result |
| `07` | CIT | corporate income tax |
| `08` | CAPEX | `is_pnl false` |
| `09` | OUT OF P&L | `is_pnl false` |
| `91` | GM | `NET SALES - CS` |
| `92` | NM | `GM - OCS` |
| `93` | CM | `NM - OPEX - OIE` |
| `94` | IBT | `CM - FR` |
| `95` | NI | `IBT - CIT` |

Read in order, those five formulas are a single chain, and the chain is the report.

## D1 — The computed lines are structure, not a parsed string

`formula` holds `'NET SALES - CS'`: category **names**, resolved by nothing, validated by
nothing, and unique by nothing. Renaming a category silently produces a report whose gross
margin is zero. Parsing the string at read time makes a typo in a text column a numeric
error in a customer's accounts.

So the arithmetic is a table in Go, over codes:

```go
var computed = []line{
    {code: "91", label: "GM",  plus: []string{"01"}, minus: []string{"02"}},
    {code: "92", label: "NM",  plus: []string{"91"}, minus: []string{"03"}},
    {code: "93", label: "CM",  plus: []string{"92"}, minus: []string{"04", "05"}},
    {code: "94", label: "IBT", plus: []string{"93"}, minus: []string{"06"}},
    {code: "95", label: "NI",  plus: []string{"94"}, minus: []string{"07"}},
}
```

`formula` stays, as the label a screen prints beside the line — it is good prose and a
reader wants it. What it stops being is the thing the computer follows.

A test asserts the two agree: every name in every stored `formula` resolves to a category,
and the operands it names are the operands the table uses. Drift then fails in CI rather
than in a report.

**Alternative rejected:** make `formula` reference codes and evaluate it. It moves the
fragility rather than removing it, and an expression language in a text column is a parser
nobody signed up to maintain.

## D2 — `pnl_section` is filled, because deriving it is right until it is not

Today it is NULL on all 46 seeded rows, and the section of any category is derivable: it is
the first two characters of the code, which is its level-1 ancestor by construction.

That derivation is true of the seeded tree and not guaranteed of an organisation's own
leaves. A customer's category hangs under a shared parent, so its code begins with that
parent's — but nothing in the schema says so, and the report would be reading a convention
rather than a fact.

Migration 009 fills the column for every row, shared and tenant, and a trigger keeps a new
row's section equal to its parent's. The report then reads one column.

Filling it is also what makes the column honest: a column created in 005, read by nothing
and NULL everywhere is not a decision anybody made, it is a decision nobody finished.

## D3 — The sign convention, settled here because three changes need it

`amount_minor` is **signed**: money in is positive, money out is negative. Every sum in the
system is then a plain sum, and a section total needs no `CASE`.

That is not the only possible convention — storing magnitudes and reading `direction` also
works — but it is the one already assumed in two places. 2.6 pairs internal transfers on
"opposite signs". The review queue orders by `abs(sum(...))`, which is meaningless if every
row is positive.

The consequence for the P&L is that a cost section sums to a negative number, and the
report **prints costs as positive** with the sign carried by the line's role rather than by
the figure. An owner reading "OPEX −412,000" beside "NET SALES 1,200,000" is reading a
spreadsheet, not a report. The formula table above is written in the stored signs, so
`CM = NM - OPEX - OIE` is `NM + OPEX + OIE` in arithmetic — which is exactly the kind of
inversion that belongs in one tested place and nowhere else.

`direction` becomes a generated column, as Track A proposes, with the vocabulary the rules
already use:

```sql
direction text GENERATED ALWAYS AS
    (CASE WHEN amount_minor >= 0 THEN 'income' ELSE 'expense' END) STORED
```

`>=` rather than `>`: a zero-amount row is a fee waiver or a correction, and calling it an
expense is a guess. Anything else breaks the 71 seeded rules, the classifier's field switch
and the proto's stated vocabulary at once.

## D4 — A line is computed from one `source_kind`, and the report says which

`ARCHITECTURE.md` §5.1 calls this the single most likely way the product prints a wrong
number. The request names the basis; every figure comes from rows of that kind only; the
basis is a label at the top of the table and not a footnote.

Where the organisation has both kinds for a period, the other side is not summed and not
silently dropped: the response carries its total separately, as reconciliation evidence for
the drill-down 4.2 opens. Mixing them requires a confirmed D4 link, which is 2.6's, and
until then a line that would need one is blocked rather than guessed.

## D5 — The calculation takes rows, not a database

```go
func Compute(rows []Row, spec Spec) (Report, error)
```

`Row` is what one classified transaction contributes: a category code, a period, a signed
minor amount, a source kind. No identifiers, no pointers, nothing to read further.

This is the same property the classifier has and for the same reason: a report that is a
pure function of its inputs can be reproduced from stored versions six months later, and an
accountant will ask. It also means the whole of the arithmetic — the chain, the percentages,
the exclusions — is tested without Postgres, and the query is tested separately for what
queries get wrong.

## D6 — What the report cannot include, it counts

Four buckets, each in money and each shown:

| | |
| --- | --- |
| **Unclassified** | no live classification. The engine did not answer and nobody has |
| **Excluded, non-P&L** | `is_pnl false`: CAPEX and OUT OF P&L |
| **Unallocated** | `requires_allocation`: known to be payroll, not yet attributed to a department |
| **Other basis** | rows of the source kind this report is not computed from |

The first is the one that matters. A P&L that sums only what was classified is smaller than
the business and looks finished. Showing the amount at risk is what the readiness check will
later turn into "partial, with named gaps", and it costs one query now.

`requires_allocation` is its own bucket rather than folded into OPEX because migration 005
marked exactly two categories with it for this reason: the amount is known to be payroll and
not known to be any department's, and attributing it would be a guess printed as a figure.

## D7 — Periods are closed intervals on `booked_on`, in the organisation's time

A period is `[from, to]` on `booked_on`, which is a date and carries no time zone —
deliberately, since a booking date has none. Columns are generated from the range and the
granularity, and a period with no rows is a column of zeros rather than an absent column: a
month that is missing from a report is indistinguishable from a month with no trade, and one
of those is a data problem.

`value_on` is not used. It is stored, and which of the two a report should use is a question
for the change that has a customer asking it.

## Risks

| Risk | Mitigation |
| --- | --- |
| The sign convention is settled here but 2.6 and the review queue already shipped against an assumption | §D3 writes it down and the generated column enforces it; the review queue's sums are already signed and its ordering already absolute |
| `direction` becoming generated breaks the 71 seeded rules | The generated vocabulary is `income`/`expense`, identical to what the rules match; a test replays the rule set against generated rows |
| Filling `pnl_section` in 009 conflicts with Track A's numbering | Named in the proposal; renumbering is free before a merge |
| The formula table and the stored `formula` drift | A test resolves every stored formula's names and compares the operands to the table |
| Costs print negative | One inversion, in `Compute`, tested against a fixture whose expected output is written the way an owner reads it |
