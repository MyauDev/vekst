## Context

Change 4.1 produces figures. This makes each one openable, and the difficulty is not the
query — it is making the query provably the same selection the figure was summed from.

## D1 — A cell is addressed by what produced it, not by a row identifier

The obvious design returns an opaque handle with each figure and takes it back. It is
tempting and it is wrong: a handle is state the server has to keep or sign, and a report is
supposed to be reproducible from nothing but its inputs.

So a cell is addressed by the same four things that computed it:

```
(entity_id, basis, period, line)
```

`line` is a category code for a section, or one of the four bucket names for an exclusion.
Nothing else is needed, because nothing else went into the sum — which is exactly the
property design §D5 of 4.1 was arranged to have.

The consequence worth stating: **the drill-down and the report share one predicate builder.**
Not two queries that agree today, one function used twice. A figure whose drill-down does not
add up to it is the specific failure this change exists to make impossible, and two
independently-written `WHERE` clauses are how it happens.

A test asserts it directly: for every non-zero cell in a fixture report, the sum of its
drill-down equals the cell.

## D2 — A computed line opens to its operands, not to transactions

GM has no transactions. It is `NET SALES - CS`, and both of those have transactions.

Opening GM returns the two lines it is made of and their figures, with each of *those*
openable in turn. That is one extra level of indirection and it is the honest one: a
drill-down that showed GM's "transactions" would have to invent a set, and the set it would
invent — everything in NET SALES and CS — is exactly what a reader would misread as a single
category.

The response says which kind of answer it is, so the screen renders a list of rows or a list
of lines without guessing.

## D3 — Every row carries why it is on that line

Four fields, all already stored, none currently shown:

| | |
| --- | --- |
| `engine_layer` | `L0` memory, `L0.5` a regulated code, `L1` a rule, `human` a person |
| `confidence` | 1.00, 0.99, 0.95, or a person's certainty |
| `evidence` | the counterparty-key tier that matched, or the rule's scope |
| `decided_by` | present exactly when a person decided |

Layer and evidence are the pair that answers "why". "Because a Belarusian country rule
matched this wording" and "because you told us in March" are different claims about the same
figure, and a reviewer trusts them differently.

`matched_rule_priority` is deliberately **not** returned. It is an internal ordering number,
it means nothing to a reader, and exposing it invites a client to reason about rule order it
has no business knowing.

## D4 — `line_no`, and why it is not optional

A customer checking a figure opens their own file. A drill-down that says "412,000 across
these nine payments" without saying which line of `march.csv` each came from asks them to
match on amount and date and hope.

`ARCHITECTURE.md` §4a.1 already keys the validation error report to the line in the
**original** file, and `ingest.Row.LineNo` already carries it — `TestLineNumbersReferToTheOriginalFile`
pins that it is the file's line and not the parsed row's index. It is lost when a row is
persisted, and this is where that costs something.

Stored as `line_no integer NOT NULL` beside `batch_id`: the pair is the row's provenance, and
either alone is half an answer.

## D5 — The reconciliation strip is an assertion, not a summary

`DESIGN.md` §8 puts opening, in, out, transfers and closing at the foot of the table. What
makes it worth building is that it is checkable:

```
opening + in - out - transfers = closing
```

When it does not hold, the report says so rather than printing five numbers that do not add
up. Until 2.6 supplies confirmed internal transfers the term is zero and the strip says that
too — a zero with a reason beside it, not a blank.

Opening and closing balances are not stored anywhere yet. For the Demo they are derived from
the rows themselves, which makes the identity trivially true and the strip merely
informative; it becomes an actual check when a statement's own declared balances are stored,
and change 2.3's balance check is where those arrive. The strip is built now because the
screen has a place for it and because deriving it later against a different definition is
worse than deriving it now against a stated one.

## D6 — Paging is by a stable key, not an offset

A drill-down can be thousands of rows and, unlike the review queue, it is not being emptied
while somebody reads it. `(booked_on, id)` is stable, unique and already indexed, so a cursor
over it neither skips nor repeats.

The review queue uses an offset for the opposite reason — its list shrinks as it is worked,
so a cursor points into a set that no longer exists. The two are different problems and it is
worth the asymmetry.

## Risks

| Risk | Mitigation |
| --- | --- |
| The drill-down and the figure disagree | One predicate builder, used by both, with a test summing every cell's drill-down back to the cell |
| `line_no` never arrives from the parser | It already exists as `ingest.Row.LineNo`; the loss is at the persist boundary, which 2.5 owns |
| The strip's identity holds trivially and reads as a real check | Stated in §D5 and in the response: derived balances are labelled as derived |
| A computed line's drill-down invents a transaction set | §D2 returns operand lines instead, and the response says which kind it is |
