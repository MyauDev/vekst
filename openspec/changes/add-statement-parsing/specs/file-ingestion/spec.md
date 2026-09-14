# file-ingestion Specification

## Purpose

Reading a customer's file. This capability answers one question — *what does this file say* —
and deliberately answers no others: whether what it says is complete, whether it has been
seen before, and what it means are three other changes. Its one hard promise is that
everything it reports can be traced back to a line a person can find in the file they
uploaded.

## ADDED Requirements

### Requirement: A file is read by the parser for the bank that wrote it

The system SHALL select a parser by inspecting the file's own contents, SHALL refuse the
file when no parser recognises it, and SHALL refuse the file when more than one parser
claims it. A refusal SHALL name the formats that were tried.

#### Scenario: A recognised export is read by its own parser

- **WHEN** a Priorbank export is parsed
- **THEN** a statement is returned
- **AND** it records which parser produced it

#### Scenario: An unrecognised file is refused with something actionable

- **WHEN** a file no parser recognises is parsed
- **THEN** the file is refused
- **AND** the refusal lists the formats that were tried

#### Scenario: Two parsers claiming one file is an error, not a race

- **WHEN** more than one parser reports that it recognises a file
- **THEN** the file is refused
- **AND** every parser that claimed it is named
- **AND** no parser is allowed to win by being registered first

#### Scenario: A parser does not claim a file from another bank

- **WHEN** a statement from a different bank is offered to the Priorbank parser
- **THEN** it is not recognised

### Requirement: The encoding of a CIS export is determined from its bytes

The system SHALL determine whether a file is UTF-8, windows-1251 or CP866 without relying
on a declaration, and SHALL decode it to UTF-8. A byte sequence that cannot be represented
SHALL become a replacement character rather than a failure, and detecting that condition
SHALL be available to validation.

#### Scenario: A windows-1251 export is read as Cyrillic text

- **WHEN** a windows-1251 file is parsed
- **THEN** its Cyrillic text is decoded correctly
- **AND** no declaration in the file was required

#### Scenario: A wrong guess is visible rather than fatal

- **WHEN** decoding produces a replacement character
- **THEN** parsing still returns a result
- **AND** the condition is detectable, so validation can reject the batch with a line number

### Requirement: An amount never passes through a floating-point value

The system SHALL convert a bank's rendering of a number — with space, non-breaking space or
dot grouping, and comma or dot as the decimal mark — into `int64` minor units scaled by the
currency's own exponent, without an intermediate floating-point representation.

#### Scenario: CIS grouping and a comma decimal mark

- **WHEN** an amount written `2 009,07` is read for a currency with two minor digits
- **THEN** the stored value is 200907 minor units

#### Scenario: Either separator convention is unambiguous

- **WHEN** amounts written `2.009,07` and `2,009.07` are read
- **THEN** both yield the same minor-unit value
- **AND** the separator appearing last is treated as the decimal mark

### Requirement: Every parsed row names its line in the original file

The system SHALL record, for each row, the 1-based line number it occupied in the file as
uploaded — not its index among the rows that survived parsing — and that number SHALL
survive into every later stage.

#### Scenario: A row points at the line a person would open

- **WHEN** a statement with a multi-line preamble is parsed
- **THEN** each row's recorded line number identifies its own line in the original file
- **AND** the number is unaffected by how many preamble lines were skipped

#### Scenario: A parse failure names a line

- **WHEN** a row cannot be read
- **THEN** the error identifies the line in the original file

### Requirement: Column layout is read from the header, never assumed

The system SHALL locate columns by their header names, and SHALL read a statement's summary
rows without relying on the header's column positions.

#### Scenario: One bank, two layouts, one download

- **WHEN** a rouble account export and a currency account export from the same bank are
  parsed
- **THEN** both are read correctly
- **AND** neither depends on a fixed column position

#### Scenario: A currency account's balances are not mixed with its equivalents

- **WHEN** a currency account's summary rows are read
- **THEN** the account's own figures are taken
- **AND** the bank's base-currency equivalents are not substituted for them

### Requirement: The statement reports what the bank declared and what its rows sum to

The system SHALL retain the opening balance, the closing balance and the declared turnover
as the bank stated them, and SHALL compute the difference between the declared closing
balance and opening plus the movements actually parsed, at zero tolerance in minor units.
The capability SHALL report that difference and SHALL NOT decide what it means.

#### Scenario: A complete file reconciles exactly

- **WHEN** a statement whose rows account for every movement is parsed
- **THEN** the computed difference is zero

#### Scenario: A discrepancy is reported as a number, not a verdict

- **WHEN** the rows do not account for the declared closing balance
- **THEN** the difference is returned in minor units
- **AND** the file is not rejected by this capability
- **AND** the outcome is left to validation

### Requirement: Debit and credit are both retained, and only both non-zero is an error

The system SHALL retain both the debit and the credit column of every row. A row SHALL be
treated as malformed only when **both are non-zero**; a row carrying a zero in one of them
SHALL be ordinary.

#### Scenario: The ordinary row has a zero on one side

- **WHEN** a row carries a credit and a debit of `0,00`
- **THEN** the row is read normally
- **AND** it is not reported as having both columns populated

#### Scenario: Both sides carrying value is malformed

- **WHEN** a row carries a non-zero debit and a non-zero credit
- **THEN** the row is malformed

### Requirement: Reading a file has no side effects

The system SHALL parse using only the bytes of the file. A parser SHALL NOT read a
database, a clock or the network, so that re-reading the same bytes always yields the same
statement.

#### Scenario: The same bytes give the same statement months later

- **WHEN** a file is parsed twice at different times
- **THEN** the two statements are identical

#### Scenario: A parser observes no tenant

- **WHEN** a file is parsed
- **THEN** no tenant context is required
- **AND** no row is read from or written to any table

### Requirement: A parsed line is stored so its parse can be re-examined

The system SHALL persist one row per parsed line, keyed to the batch and the line number it
came from, under the same row-level security every tenant table carries. The stored payload
SHALL be the cells this capability extracted, not a re-encoding of the file's decoded text —
decoding is reproducible exactly from the object store's original bytes, so storing it again
adds nothing; what is not reproducible after a later change to this capability's own parsing
logic is what a given parse actually decided at the time.

This is a persistence step performed with the parser's output, not by the parser itself —
the requirement above that a parser has no side effects is unchanged.

#### Scenario: A parsed batch's lines are stored under its own tenant

- **WHEN** a batch belonging to organisation A is parsed
- **THEN** one `raw_rows` row exists per parsed line
- **AND** each carries A's `org_id` and the batch's id

#### Scenario: One organisation cannot see or write another's raw rows

- **WHEN** organisation B addresses A's batch by id, under B's own tenant context
- **THEN** no row is returned
- **AND** an attempt to insert against A's batch id under B's context is refused, because no
  row satisfies the composite foreign key `(org_id, batch_id)` for B

#### Scenario: A read without tenant context raises

- **WHEN** `raw_rows` is queried outside a tenant transaction
- **THEN** the database raises `42704`
- **AND** it does not return zero rows

#### Scenario: A line is stored once

- **WHEN** the same batch and line number are persisted a second time
- **THEN** the write is refused by a uniqueness constraint
