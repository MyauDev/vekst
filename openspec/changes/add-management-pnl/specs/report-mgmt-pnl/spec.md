# report-mgmt-pnl Specification

## Purpose

The management P&L: what a business owner reads in the first minute of a month. Sections,
the five computed lines, one column per period, and an honest account of everything the
report could not include.

## Requirements

### Requirement: A line is computed from one source kind, and the report names it

The system SHALL compute every figure from transactions of a single `source_kind`, SHALL
state which on the report itself, and SHALL NOT mix ledger and bank rows into one line
without a confirmed match.

Mixing them counts an invoice and its payment twice. This is the single most likely way the
product prints a wrong number.

#### Scenario: The basis is stated, not footnoted

- **WHEN** a report is produced
- **THEN** it carries the source kind every figure was computed from

#### Scenario: The other side is reported separately

- **WHEN** an organisation holds both ledger and bank rows for a period
- **THEN** only the requested basis contributes to the lines
- **AND** the total of the other basis is returned separately rather than dropped

### Requirement: The computed lines follow a fixed chain

The system SHALL compute GM, NM, CM, IBT and NI from section totals, in that order, and
SHALL NOT derive the arithmetic by parsing the stored `formula` text.

`formula` names categories by name. Names are not unique, not validated and not stable, so a
rename would silently produce a wrong figure rather than an error.

#### Scenario: The chain produces the expected figures

- **WHEN** a period has one transaction in each section
- **THEN** GM is NET SALES minus CS
- **AND** NM is GM minus OCS
- **AND** CM is NM minus OPEX and OIE
- **AND** IBT is CM minus FR
- **AND** NI is IBT minus CIT

#### Scenario: The stored formulas agree with the computation

- **WHEN** the stored `formula` of every computed category is resolved
- **THEN** each names categories that exist
- **AND** names the same operands the computation uses

### Requirement: Money is summed in one currency and never as a float

The system SHALL sum every figure in the organisation's base currency using integer minor
units, and SHALL return every amount with its ISO-4217 code.

#### Scenario: A currency with a different exponent contributes correctly

- **WHEN** a period contains a JPY row, whose exponent is 0, and a KWD row, whose exponent
  is 3
- **THEN** each contributes its stored base amount
- **AND** nothing is rounded

#### Scenario: No money field is floating point

- **WHEN** the report's types are inspected
- **THEN** no field carrying an amount is a floating-point type
- **AND** the only floating-point field is the percentage of revenue

#### Scenario: A percentage of no revenue is absent

- **WHEN** a period has no revenue
- **THEN** the percentage of revenue is absent
- **AND** it is neither zero nor infinite

### Requirement: The report states what it could not include

The system SHALL return, in money, the total of transactions that are unclassified, excluded
as non-P&L, awaiting allocation, or of the other basis — and every transaction in the period
SHALL fall into exactly one line or one of those totals.

A report that sums only what was classified describes a smaller business than the one that
exists, and looks finished while doing it.

#### Scenario: Unclassified money is visible

- **WHEN** a period contains transactions with no live classification
- **THEN** their total is returned as its own figure

#### Scenario: Nothing is lost between the lines and the buckets

- **WHEN** the report is produced
- **THEN** the sum of every line and every bucket equals the sum of every transaction in the
  period

#### Scenario: Unallocated payroll is its own figure

- **WHEN** transactions are classified into a category marked as requiring allocation
- **THEN** their total is shown separately
- **AND** it is not attributed to any department

#### Scenario: Non-P&L categories do not reach a line

- **WHEN** transactions are classified as CAPEX or out of the P&L
- **THEN** no P&L line includes them

### Requirement: Only the live classification of a transaction counts

The system SHALL include a transaction on the line of its live classification, and SHALL
treat a superseded or retracted classification as though it had not been made.

#### Scenario: A correction moves the money

- **WHEN** a classification is superseded by one naming a different category
- **THEN** the transaction contributes to the new line only

#### Scenario: A retraction returns the money to unclassified

- **WHEN** a review decision is undone
- **THEN** its transactions leave the line they were on
- **AND** they appear in the unclassified total

### Requirement: A report pins the versions it was produced under

The system SHALL return the `taxonomy_version`, `ruleset_version` and `engine_version` of
the classifications it summed, read from those rows rather than from configuration.

Those three strings are what make a March report reproduce in June, and an accountant will
ask.

#### Scenario: The versions come from the data

- **WHEN** a report is produced
- **THEN** the versions it reports are those recorded on the classification rows
- **AND** not constants compiled into the binary

#### Scenario: A report produced under two versions names both

- **WHEN** the classifications a report sums were made under two different engine versions
- **THEN** both are returned
- **AND** neither is chosen over the other

#### Scenario: A report that summed no classification names no version

- **WHEN** a range contains no live classification
- **THEN** the versions are empty rather than the running binary's own

### Requirement: The table is printed in one order

The system SHALL return the report's lines in a fixed order in which each computed line
immediately follows the operands it consumes, and SHALL return every section it sums and
every line it computes.

A table listing seven sections and then five results is a spreadsheet the reader has to
reassemble mentally, and the reassembly is where a misreading happens.

#### Scenario: A computed line follows its operands

- **WHEN** a report is produced
- **THEN** the lines are NET SALES, CS, GM, OCS, NM, OPEX, OIE, CM, FR, IBT, CIT, NI in that
  order

#### Scenario: Nothing computed goes unprinted

- **WHEN** the printed order is compared with what the report computes
- **THEN** every section and every computed line appears exactly once
- **AND** every exclusion total appears exactly once

### Requirement: Costs are stored signed and printed positive

The system SHALL store money out as a negative amount and SHALL present a cost line as a
positive figure, with the sign carried by the line's role.

An owner reading "OPEX −412,000" beside "NET SALES 1,200,000" is reading a spreadsheet, not
a report. The inversion is one place in the calculation and nowhere else.

#### Scenario: The store and the page disagree by construction

- **WHEN** a cost transaction is summed onto a line
- **THEN** the stored amount is negative
- **AND** the figure on the report is positive

### Requirement: Columns follow the requested granularity

The system SHALL group periods by month, quarter or year as the request names, and SHALL
label a column so that its granularity is readable from the label alone.

#### Scenario: A quarter is its months folded

- **WHEN** a quarterly report covers January to July
- **THEN** its columns are 2026-Q1, 2026-Q2 and 2026-Q3
- **AND** every month of the range contributes to exactly one column

### Requirement: Periods are complete and explicit

The system SHALL produce one column per period in the requested range, including periods
with no transactions.

A month absent from a report is indistinguishable from a month with no trade, and only one
of those is a business fact.

#### Scenario: An empty month is a zero column

- **WHEN** a period in the range contains no transactions
- **THEN** a column for it is present with zero figures

### Requirement: Reports are isolated between organisations

The system SHALL compute a report only from the requesting organisation's transactions, and
SHALL answer a request naming another organisation's entity the same way it answers one
naming an entity that does not exist.

#### Scenario: One organisation's figures contain no other's rows

- **WHEN** organisation A produces a report
- **THEN** no transaction belonging to organisation B contributes to any figure

#### Scenario: Another organisation's entity is not an oracle

- **WHEN** organisation A requests a report for an entity owned by organisation B
- **THEN** the result is empty
- **AND** it is indistinguishable from the entity not existing

#### Scenario: A non-member is refused the same way twice

- **WHEN** somebody who belongs to no such organisation requests a report
- **THEN** the request is refused
- **AND** refused identically whether the organisation exists or not

#### Scenario: Any member may read a report

- **WHEN** a member holding the viewer role requests a report
- **THEN** it is produced
