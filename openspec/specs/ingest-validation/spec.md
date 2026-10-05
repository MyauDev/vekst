# ingest-validation Specification

## Purpose

The stage between reading a file and believing it. Correctness is decided per row and is
never negotiable. Completeness is decided per file and may be accepted by a person who puts
their name to the reason. Everything that follows — every transaction, every classification,
every report — depends on this stage having said yes.

## Requirements

### Requirement: Every row is checked for correctness, and a failure rejects the file

The system SHALL check each parsed row for a plausible date, an amount that parses to
`int64` minor units, a valid ISO-4217 currency, debit and credit not both non-zero, the
absence of replacement characters, a non-empty description, and a resolvable account. If
any row fails any of these, the outcome SHALL be `rejected` and no transaction SHALL be
stored.

#### Scenario: One unparsable amount rejects the file

- **WHEN** a 3,182-row file contains one amount that does not parse in the given number
  locale
- **THEN** the outcome is `rejected`
- **AND** no transaction from that file is stored
- **AND** the error names the field and the line in the original file

#### Scenario: A row with a zero on one side is ordinary

- **WHEN** a row carries a credit and a debit of `0,00`, as every real bank export does
- **THEN** the row passes the debit/credit check
- **AND** the non-zero side determines the sign

#### Scenario: Both sides carrying value is a correctness failure

- **WHEN** a row carries a non-zero debit and a non-zero credit
- **THEN** the row fails correctness
- **AND** the file is rejected

#### Scenario: A replacement character is treated as corruption, not as text

- **WHEN** a row contains U+FFFD
- **THEN** the row fails correctness
- **AND** the reported code says the charset detection was wrong

#### Scenario: A correctness failure can never be accepted

- **WHEN** an approver attempts to override a batch whose outcome is `rejected`
- **THEN** the attempt is refused
- **AND** it is refused by the database as well as by the application

### Requirement: The file is checked for completeness, and the balance check is exact

The system SHALL check that the opening balance plus the sum of movements equals the
closing balance, **with zero tolerance**, in minor units. It SHALL also check the declared
row count, continuous coverage of the declared period, one currency per account within the
file, and the absence of duplicate bank references. A completeness failure SHALL produce
`valid_with_warnings`, not `rejected`.

#### Scenario: A file that reconciles exactly is clean

- **WHEN** opening plus movements equals closing to the minor unit
- **THEN** the balance check is recorded as passed
- **AND** the outcome is `valid` if no other warning applies

#### Scenario: A discrepancy of one minor unit is a discrepancy

- **WHEN** the sum differs from the declared closing balance by 1 minor unit
- **THEN** the balance check is recorded as failed
- **AND** the reported difference is 1
- **AND** the outcome is `valid_with_warnings`

#### Scenario: A file that declares no balances says so

- **WHEN** a file declares neither an opening nor a closing balance
- **THEN** the balance check is recorded as neither passed nor failed
- **AND** the customer is told the check could not run, rather than that it failed

#### Scenario: The balance is checked in the file's own currency

- **WHEN** a file in PLN is validated for an organisation reporting in EUR
- **THEN** the balance is reconciled in PLN
- **AND** no conversion occurs before the comparison

### Requirement: A completeness warning may be accepted by a person, with a reason

The system SHALL allow a warning to be overridden, SHALL record who overrode it, when, and
the reason they gave, and SHALL require that reason to be non-trivial. The override SHALL
be written once and SHALL NOT be recomputed.

#### Scenario: An accepted warning is attributable

- **WHEN** a warning is overridden
- **THEN** the person, the time and the reason are stored
- **AND** an empty or trivial reason is refused

#### Scenario: An override cannot be revised silently

- **WHEN** a second override is attempted on the same batch
- **THEN** it is refused
- **AND** the original reason and time are unchanged

#### Scenario: An accepted warning follows the numbers into the report

- **WHEN** a report covers a period containing transactions from an overridden batch
- **THEN** that batch, its reason and its balance result are available to the report
- **AND** a report that does not surface them fails its own test

### Requirement: Every outcome is recorded, including a rejection

The system SHALL write one validation record per batch, whatever the outcome, holding the
outcome, the row, error and warning counts, the balance result and the error report. The
counts and the outcome SHALL NOT be able to disagree.

#### Scenario: A rejected file is still explainable a week later

- **WHEN** a customer asks why an import failed six days after it failed
- **THEN** the error report is still stored
- **AND** it lists each failing line by its number in the original file

#### Scenario: An inconsistent record cannot be stored

- **WHEN** a record is written as `valid` with a non-zero error count
- **THEN** the write is rejected by a check constraint

#### Scenario: One batch has one validation

- **WHEN** a second validation is written for a batch that already has one
- **THEN** the write is rejected by a unique constraint

### Requirement: The error report carries codes, not sentences

The system SHALL store and return machine-readable codes, field names, line numbers and
raw source text. It SHALL NOT store or return rendered human-readable messages.

#### Scenario: The report holds no prose

- **WHEN** the stored report is inspected
- **THEN** every value is a code, a number or raw source text from the file
- **AND** no value is a sentence in any language

#### Scenario: A malformed file cannot produce an unbounded report

- **WHEN** every row of a large file fails
- **THEN** the stored report is truncated
- **AND** it carries the total count of failures

### Requirement: Validations are isolated between organisations

The system SHALL apply row-level security to validation records, with `FORCE ROW LEVEL
SECURITY`, and SHALL raise rather than return an empty result when no tenant context is
set.

#### Scenario: One organisation cannot read another's error report

- **WHEN** a member of organisation A reads a validation belonging to B
- **THEN** the read returns nothing
- **AND** the refusal is identical whether that validation exists or not

#### Scenario: A read without tenant context raises

- **WHEN** a validation query runs outside a tenant transaction
- **THEN** the database raises `42704`
- **AND** it does not return zero rows
