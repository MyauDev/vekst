# transaction-ledger Specification

## Purpose

The canonical record of what a business actually moved: one row per payment or posting, in
the currency it happened in and in the currency the report is printed in, with the
classification of each row stored beside it and never overwritten.

## Requirements

### Requirement: One row per payment or posting

The system SHALL store a bank payment as one row with a NULL `document_ref` and a
`posting_no` of 0, and a ledger document of N postings as N rows sharing one `document_ref`
with `posting_no` 1…N. The system SHALL NOT store a document as one row with its line items
in a JSONB column.

This is what lets a bank payment link to the postings of the invoice it settles, and it is
what keeps every report query from being a JSONB unnest.

#### Scenario: A bank payment is one row

- **WHEN** a bank transaction is stored
- **THEN** its `document_ref` is NULL and its `posting_no` is 0

#### Scenario: A ledger document keeps its postings

- **WHEN** a ledger document with five postings is stored
- **THEN** five rows exist sharing one `document_ref`
- **AND** their `posting_no` values are 1 through 5

#### Scenario: A half-stated grain is rejected

- **WHEN** a row is written with a `document_ref` and a `posting_no` of 0, or with no
  `document_ref` and a `posting_no` above 0
- **THEN** the write is rejected by a check constraint

### Requirement: A transaction's source kind is fixed and cannot drift from its batch

The system SHALL store `source_kind` on every transaction as `ledger` or `bank`, SHALL keep
it equal to the source kind of the batch that produced the row, and SHALL reject a row that
disagrees.

A report line is computed from one source kind. Mixing them counts an invoice and its
payment twice, which is the single most likely way this product prints a wrong number.

#### Scenario: A row cannot claim a different kind from its batch

- **WHEN** a transaction is written whose `source_kind` differs from its batch's
- **THEN** the write is rejected

#### Scenario: Transactions can be read by source kind alone

- **WHEN** the transactions for a report are read
- **THEN** they can be filtered to one `source_kind` without joining the batch

### Requirement: Every transaction names the import that produced it

The system SHALL require a `batch_id` on every transaction, referencing a batch owned by the
same organisation.

A row that cannot say where it came from cannot be counted, corrected or withdrawn as part
of the import that introduced it, and the user-facing count is per import.

#### Scenario: A transaction cannot exist without a batch

- **WHEN** a transaction is written with no `batch_id`
- **THEN** the write is rejected

#### Scenario: A batch belonging to another organisation is not reachable

- **WHEN** organisation A writes a transaction naming a batch owned by organisation B
- **THEN** the write is rejected

### Requirement: Money is stored as minor units with its currency, and a conversion is recorded whole

The system SHALL store every amount as an integer count of minor units together with an
ISO-4217 code, never as a floating-point number. Where an amount is converted to the
organisation's base currency, the system SHALL store the rate, the rate's date, the
converted amount and the base currency together, or none of them.

#### Scenario: An amount survives a currency with no minor units

- **WHEN** a JPY amount, whose exponent is 0, is written and read back
- **THEN** the value is unchanged
- **AND** no rounding occurred

#### Scenario: An amount survives a currency with three minor digits

- **WHEN** a KWD amount, whose exponent is 3, is written and read back
- **THEN** the value is unchanged

#### Scenario: A partial conversion is rejected

- **WHEN** a row is written with some but not all of the rate, the rate date, the converted
  amount and the base currency
- **THEN** the write is rejected by a check constraint

#### Scenario: An unconverted row states that it is unconverted

- **WHEN** a row is stored in the organisation's own base currency
- **THEN** all four conversion columns are NULL
- **AND** a report can tell that apart from a conversion that failed to happen

#### Scenario: No money value reaches the application as a float

- **WHEN** the generated types for this table are inspected
- **THEN** no column carrying an amount is a floating-point type

### Requirement: Normalised text carries the version that produced it

The system SHALL store `description_norm` and `counterparty_key` alongside the
`normalize_version` that produced them, and SHALL send that version with any classification
request built from the row.

#### Scenario: The version travels with the value

- **WHEN** a batch is built for classification from stored transactions
- **THEN** the request's `normalize_version` is the one recorded on the rows
- **AND** not a value read from configuration

#### Scenario: A changed normaliser is a backfill, not a reinterpretation

- **WHEN** `core/internal/normalize` changes and its version is bumped
- **THEN** existing rows keep the old version until they are rewritten
- **AND** rules written for the old version continue to match them

### Requirement: Classifications are append-only and exactly one is live

The system SHALL never update or delete a classification. A correction SHALL insert a new
row and point the previous one at it. At most one classification per transaction SHALL be
live at any time, enforced by a unique index rather than by convention.

#### Scenario: A correction supersedes rather than overwrites

- **WHEN** a transaction's classification is corrected
- **THEN** a new row is inserted
- **AND** the previous row's `superseded_by` names it
- **AND** the previous row's category is unchanged

#### Scenario: The application cannot rewrite a classification

- **WHEN** the application attempts to update a classification's category or delete a row
- **THEN** the statement is refused by the grants

#### Scenario: Two live classifications cannot exist

- **WHEN** a second classification is inserted for a transaction that already has a live one
- **THEN** the write is rejected by a unique index

### Requirement: Every classification records what produced it

The system SHALL store `engine_layer`, `taxonomy_version`, `ruleset_version`,
`engine_version` and `normalize_version` on every classification row.

Those strings are what make a March report reproduce in June, and an accountant will ask.

#### Scenario: A report can pin its inputs

- **WHEN** a report is computed from a set of classifications
- **THEN** every version that contributed to them is readable from the rows themselves

#### Scenario: A human decision names the human

- **WHEN** a classification is written with layer `human`
- **THEN** `decided_by` is populated
- **AND** a row with layer `human` and no decider is rejected

### Requirement: A classification can never target a section or a computed line

The system SHALL restrict a classification to a category that is a visible leaf and is not
computed, and SHALL raise the same error for a category belonging to another organisation as
for one that does not exist.

#### Scenario: A computed line is refused

- **WHEN** a classification names GM, NM, CM, IBT or NI
- **THEN** the write is rejected

#### Scenario: Another organisation's category is not an existence oracle

- **WHEN** organisation A classifies against a category owned by organisation B
- **THEN** the write is rejected
- **AND** the error is identical to the error raised for a category that does not exist

### Requirement: Transactions and classifications are isolated between organisations

The system SHALL apply row-level security with `FORCE ROW LEVEL SECURITY` to
`transactions`, `classifications` and `import_batches`, and SHALL scope every uniqueness
constraint by `org_id`.

#### Scenario: One organisation cannot read another's transactions

- **WHEN** the tenant context is organisation A and a query selects every row
- **THEN** no row owned by organisation B appears

#### Scenario: One organisation cannot classify another's transaction

- **WHEN** organisation A inserts a classification naming a transaction owned by B
- **THEN** the write is rejected

#### Scenario: A write that affects no row is an error, not a silent success

- **WHEN** a single-row update matches no row because of the tenant policy
- **THEN** the caller receives `ErrNoRowsAffected`

### Requirement: Duplicate detection is possible from the first row stored

The system SHALL store a non-null `dedup_hash` on every transaction, computed from the row's
own content, with a uniqueness constraint scoped by `org_id`.

The hash is stored before anything consumes it, because adding a unique index to a table
that already holds duplicates is an incident rather than a migration.

#### Scenario: Identical content hashes identically

- **WHEN** two transactions with the same account, date, amount, currency, normalised
  description, bank reference and grain are hashed
- **THEN** the hashes are equal

#### Scenario: A difference in any hashed field separates them

- **WHEN** two transactions differ in exactly one hashed field
- **THEN** their hashes differ

#### Scenario: A duplicate cannot be stored twice

- **WHEN** a transaction is written whose `dedup_hash` already exists for that organisation
- **THEN** the write is rejected by a unique index
