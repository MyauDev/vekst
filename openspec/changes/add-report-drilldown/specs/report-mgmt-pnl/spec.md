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
