# review-queue Specification

## Purpose

Where a person settles what the engine could not, one counterparty at a time, biggest amount
first — and where every decision becomes memory, so the same counterparty is never asked
about twice.

## Requirements

### Requirement: A transaction needing review is one with no live classification

The system SHALL derive the queue from the absence of a live classification rather than from
a stored per-transaction state.

The engine does not emit a low-confidence proposal and mark it; below the threshold it emits
nothing. A second representation of "needs review" would duplicate a fact
`classifications` already holds, and the duplicate drifts silently: a row marked resolved
with no classification is absent from both the queue and the report.

#### Scenario: An unclassified transaction is in the queue

- **WHEN** a transaction has no classification whose `superseded_by` is NULL
- **THEN** it appears in the queue

#### Scenario: A classified transaction is not

- **WHEN** a live classification exists for a transaction
- **THEN** it does not appear in the queue
- **AND** no per-transaction review state had to be set for that to be true

### Requirement: The queue is grouped by counterparty and ordered by amount

The system SHALL group the queue by `counterparty_key`, SHALL order groups by the absolute
total in the organisation's base currency descending, then by row count descending, then by
key, and SHALL return the same order for two reads of unchanged data.

Ordering by amount is what makes the promised fifteen minutes reachable: the customer settles
the €40,000 line before the €4 one. The deterministic tiebreak is what keeps the ground from
moving under a person working by keyboard.

#### Scenario: The largest amount is first

- **WHEN** the queue is read
- **THEN** the group with the largest absolute base-currency total comes first

#### Scenario: A refund sorts with a payment of the same size

- **WHEN** one group totals −40,000 and another +40,000
- **THEN** they sort adjacently rather than at opposite ends

#### Scenario: Amounts in different currencies compare in the base currency

- **WHEN** a group of JPY rows and a group of EUR rows are ordered
- **THEN** the comparison uses each row's stored base amount
- **AND** never its face value in its own currency

#### Scenario: Two reads agree

- **WHEN** the queue is read twice with no intervening write
- **THEN** the groups are in the same order

#### Scenario: Unidentified counterparties are shown, not hidden

- **WHEN** transactions exist whose counterparty could not be identified
- **THEN** they form a group like any other and are ordered by amount
- **AND** the queue does not report completion while they remain

### Requirement: One decision resolves every row from a counterparty

The system SHALL apply a single decision to every queued transaction sharing that
counterparty key, SHALL write a classification for each with layer `human`, SHALL record one
decision naming how many rows and what total it covered, and SHALL do all of it in one
transaction or none of it.

#### Scenario: Five hundred rows, one keystroke

- **WHEN** a counterparty with 500 queued transactions is resolved
- **THEN** 500 classifications are written, each with layer `human`
- **AND** exactly one decision row records the count and the total
- **AND** the group leaves the queue

#### Scenario: A partial failure leaves nothing

- **WHEN** any write in a resolution fails
- **THEN** no classification, no vendor row and no decision is persisted

#### Scenario: A decision covers only what was read

- **WHEN** a transaction for the same counterparty is imported after a decision was made
- **THEN** the decision's recorded count is unchanged
- **AND** the new row is answered by vendor memory rather than by the old decision

### Requirement: A categorisation writes vendor memory, and other outcomes do not

The system SHALL write a `vendors` row when the outcome is a category, and SHALL NOT write
one when the outcome is an internal transfer, a non-P&L marking or a skip.

Memory answers "which category is this counterparty", so an outcome that is not a category
has nothing to remember. Writing one would make L0 answer next month with a category that
does not exist.

#### Scenario: The learning loop closes

- **WHEN** a counterparty is categorised, and a later import brings a new transaction for it
- **THEN** that transaction is classified by L0 from vendor memory
- **AND** it does not reach the queue

#### Scenario: An internal transfer leaves no memory

- **WHEN** a group is marked as an internal transfer
- **THEN** no `vendors` row is created for that key

### Requirement: Only a resolver may resolve

The system SHALL permit `owner`, `admin` and `approver` to resolve and undo, SHALL permit
`viewer` to read the queue, and SHALL refuse a `viewer`'s write with an error code.

The check SHALL live in the application, not in a row-level security policy. RLS is
containment: it guarantees a transaction bound to one organisation touches only its rows,
and cannot know whether the caller was entitled to act within it.

#### Scenario: A viewer reads but does not write

- **WHEN** a `viewer` lists the queue
- **THEN** the groups are returned
- **AND** a resolve or an undo from the same session is refused with a code

#### Scenario: Authorization is not a policy

- **WHEN** the row-level security policies are inspected
- **THEN** none of them references a membership role

### Requirement: An undo reverses a decision without erasing it

The system SHALL supersede every classification a decision wrote, SHALL remove the vendor
memory it created, SHALL stamp the decision as undone with who and when, and SHALL NOT
delete the decision or any classification.

#### Scenario: The group returns to the queue

- **WHEN** a decision is undone
- **THEN** its transactions have no live classification again
- **AND** the group reappears in the queue in its original position

#### Scenario: The history survives

- **WHEN** a decision is undone
- **THEN** the decision row still exists, carrying who undid it and when
- **AND** both generations of every affected classification remain readable

#### Scenario: Memory does not outlive the decision that made it

- **WHEN** a categorisation is undone
- **THEN** the vendor row it wrote no longer answers L0 for that counterparty

### Requirement: The queue is isolated between organisations

The system SHALL apply row-level security with `FORCE ROW LEVEL SECURITY` to
`review_decisions`, and SHALL scope every uniqueness constraint by `org_id`.

#### Scenario: One organisation cannot see another's queue

- **WHEN** the tenant context is organisation A
- **THEN** no group containing organisation B's transactions is returned

#### Scenario: One organisation cannot resolve another's counterparty

- **WHEN** organisation A resolves a counterparty key that exists only in organisation B
- **THEN** no row is affected
- **AND** the outcome is indistinguishable from the counterparty not existing

### Requirement: One live decision per counterparty

The system SHALL permit at most one decision per counterparty key and key version that has
not been undone, enforced by a unique index.

A correction is an undo followed by a new decision, not a second decision sitting beside the
first.

#### Scenario: A second live decision is rejected

- **WHEN** a decision is written for a counterparty that already has a live one
- **THEN** the write is rejected by a unique index

#### Scenario: A resolved counterparty can be decided again after an undo

- **WHEN** a decision is undone and the counterparty is resolved differently
- **THEN** the new decision is accepted
