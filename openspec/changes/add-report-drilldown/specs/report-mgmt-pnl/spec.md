# report-mgmt-pnl Specification — drill-down

## Purpose

Every figure in the report opens to the transactions it was summed from, each carrying why
it is on that line. A number nobody can check is a number nobody can trust.

## Requirements

### Requirement: A drill-down sums to the figure it opens

The system SHALL compute a cell and its drill-down from one selection, so that the rows
returned for a cell sum to that cell exactly.

Two independently written predicates that agree today are the mechanism by which a report
and its evidence drift apart, and the drift is silent.

#### Scenario: Every cell adds up

- **WHEN** each non-zero cell of a report is opened
- **THEN** the sum of the returned rows equals the cell

#### Scenario: A cell is addressed by what produced it

- **WHEN** a drill-down is requested
- **THEN** it is identified by entity, basis, period and line
- **AND** no server-held handle or token is required

#### Scenario: A column clipped by the requested range opens to what it counted

- **WHEN** a quarterly report is requested for a range starting mid-quarter
- **THEN** opening that quarter's cell returns only the months in the range
- **AND** the rows it returns sum to the figure

#### Scenario: A cell that does not exist is refused

- **WHEN** a line or a period that is not part of the report is opened
- **THEN** the request is refused with an error code
- **AND** it is not answered with an empty page

### Requirement: A computed line opens to its operands

The system SHALL return the lines a computed figure is made of, rather than a set of
transactions, and SHALL say which kind of answer it gave.

GM has no transactions; it is NET SALES minus CS. Inventing a transaction set for it would
present two categories as one.

#### Scenario: Opening GM

- **WHEN** the GM line is opened
- **THEN** the response names NET SALES and CS with their figures
- **AND** it states that it answered with lines rather than transactions

#### Scenario: Opening a section

- **WHEN** a section line is opened
- **THEN** the response carries transactions
- **AND** it states that it answered with transactions

### Requirement: Every row says why it is on that line

The system SHALL return the engine layer, the confidence and the evidence stored on each
row's live classification, and the deciding person where one decided.

#### Scenario: A machine answer and a human one are distinguishable

- **WHEN** a line contains a row classified by a rule and a row classified by a person
- **THEN** their layers differ
- **AND** the human row names who decided

#### Scenario: Internal ordering is not exposed

- **WHEN** a drill-down row is returned
- **THEN** it carries no rule priority

### Requirement: A row can be found in the file it came from

The system SHALL return the line number in the original file and the batch for every row.

The validation error report is already keyed by the original file's line number. A figure a
customer cannot trace back to a row in their own spreadsheet is one they cannot verify.

#### Scenario: The line number is the file's

- **WHEN** a row is opened from a report
- **THEN** its line number is the line in the uploaded file
- **AND** not the index of the parsed row

### Requirement: The excluded money is openable too

The system SHALL allow the unclassified, non-P&L, unallocated and other-basis totals to be
opened in the same way as a line.

Counting what a report could not include turns it into work only if somebody can look at it.

#### Scenario: Unclassified money opens

- **WHEN** the unclassified total is opened
- **THEN** the transactions behind it are returned
- **AND** none of them has a live classification

#### Scenario: A retracted classification returns its row to the unclassified bucket

- **WHEN** a review decision is undone
- **THEN** its transactions appear under unclassified
- **AND** they no longer appear under the line they were on

### Requirement: The reconciliation strip states its own arithmetic

The system SHALL return opening, in, out, transfers and closing for the period, SHALL report
when the identity does not hold, and SHALL label a derived balance as derived.

#### Scenario: The identity holds

- **WHEN** the strip is computed
- **THEN** opening plus in minus out minus transfers equals closing

#### Scenario: A broken identity is reported

- **WHEN** the terms do not reconcile
- **THEN** the response says so rather than presenting five figures that do not add up

#### Scenario: Transfers are zero with a reason

- **WHEN** no confirmed internal transfers exist
- **THEN** the term is zero
- **AND** the response states that none are confirmed rather than leaving it blank

### Requirement: A drill-down is isolated between organisations

The system SHALL return no transaction belonging to another organisation, and SHALL answer a
request naming another organisation's entity the same way it answers one naming an entity
that does not exist.

#### Scenario: One organisation cannot open another's figures

- **WHEN** organisation A opens a cell naming organisation B's entity
- **THEN** the result is empty
- **AND** it is indistinguishable from the entity not existing

### Requirement: Paging neither skips nor repeats

The system SHALL page a drill-down by a stable key rather than by an offset.

#### Scenario: A thousand rows page cleanly

- **WHEN** a drill-down of a thousand rows is read page by page
- **THEN** every row appears exactly once

### Requirement: A row shows its own amount and contributes its converted one

The system SHALL return, for every drill-down row, the amount as the source recorded it and
the amount in the organisation's base currency, and SHALL sum a cell from the converted
amounts only.

A person checking a figure of 7,000 against a line reading 1,000,000 in their own file needs
to see both numbers to believe either. Summing face values across currencies is arithmetic on
incompatible units.

#### Scenario: A foreign row carries both amounts

- **WHEN** a cell containing a row in a non-base currency is opened
- **THEN** the row shows the amount the statement recorded, in its own currency
- **AND** the cell's total is the sum of the base amounts

#### Scenario: An unconverted row claims no conversion

- **WHEN** a row is already in the organisation's base currency
- **THEN** its converted amount is absent rather than a conversion at a rate of one

### Requirement: No money in a drill-down is a floating-point number

The system SHALL carry every drill-down amount as integer minor units with an ISO-4217 code.

#### Scenario: Confidence is the only float

- **WHEN** the drill-down types are inspected
- **THEN** no field carrying an amount is floating point
- **AND** the only floating-point field is the classification's confidence

### Requirement: Provenance is complete or the row is not written

The system SHALL store the original file's line number on every transaction, and SHALL refuse
a transaction that carries none rather than storing a placeholder.

A sentinel in this column is a number a customer reads as a line in their own file. A missing
provenance that says so is better than a confident wrong one.

#### Scenario: A transaction with no line number is refused

- **WHEN** a transaction is written without a line number
- **THEN** the write is refused
- **AND** no placeholder value is stored in its place
