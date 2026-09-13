# dedup-and-matching Specification

## Purpose

Two things that would otherwise make the numbers wrong: the same data arriving twice, and
money the customer moved between their own accounts being read as income. Both are decided
by comparison, and comparison is never proof — so nothing here deletes anything, and every
decision it takes can be inspected and reversed.

## Requirements

### Requirement: The same file cannot be imported twice

The system SHALL refuse a file whose content already belongs to an imported batch of the
same organisation, and SHALL enforce this as a uniqueness guarantee rather than as a
look-up. A file belonging to a batch that was rejected or abandoned SHALL remain
uploadable.

#### Scenario: Two simultaneous uploads of one file

- **WHEN** the same file is uploaded twice at the same moment
- **THEN** exactly one batch reaches imported
- **AND** the other fails with `already_imported` naming the first

#### Scenario: The same file from two customers is two files

- **WHEN** two organisations upload byte-identical files
- **THEN** both import
- **AND** neither is told the other exists

#### Scenario: A corrected re-upload is allowed

- **WHEN** a file was rejected for a bad row and the customer uploads it again
- **THEN** the upload proceeds
- **AND** it is not refused as already imported

### Requirement: A repeated row is skipped, never rejected and never deleted

The system SHALL skip a row that duplicates another row in the same batch, or a row already
imported by that organisation, and SHALL import the remainder of the file. It SHALL NOT
treat a duplicate as a validation failure.

#### Scenario: A monthly re-export imports only what is new

- **WHEN** a customer uploads an export overlapping the previous month
- **THEN** the rows already held are skipped
- **AND** the new rows are imported
- **AND** the batch is not rejected

#### Scenario: A duplicate inside one file

- **WHEN** one file contains the same row twice
- **THEN** the first is imported and the second is skipped

#### Scenario: Nothing is removed

- **WHEN** any duplicate is detected
- **THEN** no stored transaction is deleted or altered

### Requirement: Every skipped row is recorded and attributable

The system SHALL record each skipped row with the line number it held in the original file,
and, where the match was against an earlier import, the transaction and batch it matched.
Counts SHALL be derived from these records.

#### Scenario: A customer can see what was dropped

- **WHEN** a batch reports skipped rows
- **THEN** each skip names its line in the original file
- **AND** a cross-batch skip names the transaction and the batch holding the original

#### Scenario: A skip does not break the balance check

- **WHEN** a file whose rows are mostly already imported is validated and imported
- **THEN** the balance check reconciles over every parsed row
- **AND** skipping rows afterwards does not make the batch fail reconciliation

#### Scenario: An indistinguishable repeat is still recorded

- **WHEN** two genuinely separate payments are identical in every hashed field
- **THEN** the second is skipped
- **AND** the skip record names its line, so the customer can find it

### Requirement: Money moved between the customer's own accounts is detected and excluded

The system SHALL detect pairs of transactions within one organisation that have opposite
signs, equal absolute base-currency amounts, booking dates within three days, and different
accounts. A detected pair SHALL be excluded from every P&L line, and a person SHALL be able
to dismiss a pair and return it to the reports.

#### Scenario: A transfer between two of the customer's accounts is not revenue

- **WHEN** an outgoing and an incoming transaction of equal amount, two days apart, on
  different accounts of one organisation are imported
- **THEN** they are paired as an internal transfer
- **AND** neither appears in any P&L line

#### Scenario: A transfer across currencies still pairs

- **WHEN** the two sides are in different currencies but equal after conversion
- **THEN** they are paired
- **AND** a later import carrying a different rate does not change the pairing

#### Scenario: A wrong pair can be undone

- **WHEN** a person dismisses a detected pair
- **THEN** both transactions return to the P&L
- **AND** who dismissed it and when are recorded together

#### Scenario: Ledger and bank rows are never paired this way

- **WHEN** a ledger row and a bank row would otherwise satisfy the rule
- **THEN** they are not paired as an internal transfer

### Requirement: Pairing is deterministic

The system SHALL produce the same pairs regardless of the order in which rows are imported,
and SHALL place each transaction in at most one pair.

#### Scenario: Three equal transfers in one week

- **WHEN** three equal outgoing and three equal incoming transfers fall within the window
- **THEN** the pairing is the same whatever order the rows arrived in
- **AND** no transaction appears in two pairs

#### Scenario: A later candidate cannot steal a paired side

- **WHEN** a new import contains a transaction nearer in date to an already-paired one
- **THEN** the existing pair is unchanged

### Requirement: Skips and transfers are isolated between organisations

The system SHALL apply row-level security to skip records and internal-transfer pairs, with
`FORCE ROW LEVEL SECURITY`, and SHALL raise rather than return an empty result when no
tenant context is set.

#### Scenario: One organisation cannot see another's skips

- **WHEN** a member of organisation A lists skipped rows
- **THEN** only A's are returned

#### Scenario: One organisation cannot dismiss another's pair

- **WHEN** a member of organisation A dismisses a pair belonging to B
- **THEN** the statement affects no row
- **AND** B's pair is unchanged

#### Scenario: A read without tenant context raises

- **WHEN** a skip or transfer query runs outside a tenant transaction
- **THEN** the database raises `42704`
- **AND** it does not return zero rows
